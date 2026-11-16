package jobs

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/spencercornish/kilnlab/internal/auth"
	"github.com/spencercornish/kilnlab/internal/db"
	"github.com/spencercornish/kilnlab/internal/firing"
	"github.com/spencercornish/kilnlab/internal/notify"
)

// Interval is how often the scheduler ticks (spec §10: ~5 minutes).
const Interval = 5 * time.Minute

// RecheckInterval is how often membership and the kiln master role are
// re-checked for signed-in users. It bounds how long someone who has left
// keeps access (sessions themselves last a week).
const RecheckInterval = time.Hour

// Kicker wakes the outbox worker; *notify.Worker satisfies it.
type Kicker interface{ Kick() }

// Runner applies Due to every candidate firing, one transaction per firing.
type Runner struct {
	pool      *pgxpool.Pool
	notifyCfg notify.Config
	kicker    Kicker
	log       *slog.Logger
	Now       func() time.Time

	// Checkers re-check membership per provider (see RecheckMembers). Empty
	// means membership is only checked at login.
	Checkers    map[db.AuthProvider]auth.MemberChecker
	lastRecheck time.Time
}

func NewRunner(pool *pgxpool.Pool, cfg notify.Config, kicker Kicker, log *slog.Logger) *Runner {
	if log == nil {
		log = slog.Default()
	}
	return &Runner{pool: pool, notifyCfg: cfg, kicker: kicker, log: log.With("component", "jobs"), Now: time.Now,
		Checkers: map[db.AuthProvider]auth.MemberChecker{}}
}

// Run ticks until ctx is cancelled, starting with an immediate tick.
func (r *Runner) Run(ctx context.Context) {
	t := time.NewTicker(Interval)
	defer t.Stop()
	for {
		if _, err := r.Tick(ctx); err != nil && ctx.Err() == nil {
			r.log.Error("jobs tick failed", "error", err)
		}
		if now := r.Now(); len(r.Checkers) > 0 && now.Sub(r.lastRecheck) >= RecheckInterval && ctx.Err() == nil {
			r.lastRecheck = now
			if err := r.RecheckMembers(ctx); err != nil && ctx.Err() == nil {
				r.log.Error("membership re-check failed", "error", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Tick runs every due job once and returns how many firings it acted on.
// One firing failing doesn't stop the others.
func (r *Runner) Tick(ctx context.Context) (int, error) {
	now := r.Now()
	r.cleanup(ctx)
	// Today is a superset cut-off for the candidate query: the studio's date is
	// never later than the UTC date plus one day. Due makes the exact call.
	ids, err := db.New(r.pool).ListJobCandidates(ctx, db.ListJobCandidatesParams{
		Now: now, Today: firing.DayOf(now.Add(24*time.Hour), time.UTC),
	})
	if err != nil {
		return 0, err
	}
	acted := 0
	var errs []error
	for _, id := range ids {
		did, err := r.process(ctx, id, now)
		if err != nil {
			r.log.Error("job failed", "firing", id, "error", err)
			errs = append(errs, err)
			continue
		}
		if did {
			acted++
		}
	}
	if acted > 0 && r.kicker != nil {
		r.kicker.Kick()
	}
	return acted, errors.Join(errs...)
}

// studioLoc is the studio's timezone, or UTC if it doesn't load.
func studioLoc(name string) *time.Location {
	if l, err := time.LoadLocation(name); err == nil {
		return l
	}
	return time.UTC
}

func rulesFrom(st db.Setting) Rules {
	return Rules{
		UnderfilledPct:    int(st.UnderfilledThresholdPct),
		MissedStartGrace:  time.Duration(st.MissedStartGraceMinutes) * time.Minute,
		MissedUnloadGrace: time.Duration(st.MissedUnloadGraceMinutes) * time.Minute,
		EscalationGrace:   time.Duration(st.EscalationGraceMinutes) * time.Minute,
	}
}

func (r *Runner) process(ctx context.Context, id int64, now time.Time) (bool, error) {
	acted := false
	err := db.InTx(ctx, r.pool, func(q *db.Queries) error {
		f, err := q.GetFiringForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		st, err := q.GetSettings(ctx)
		if err != nil {
			return err
		}
		approved, err := q.ApprovedShares(ctx, f.ID)
		if err != nil {
			return err
		}
		full := firing.ComputeFullness(f.ProposerSharePct, approved, f.FullnessOverridePct)
		loc := studioLoc(st.Timezone)
		hasOwner := r.hasResponsibleOwner(ctx, q, f)
		due := Due(Snapshot{
			Status: f.Status, HasOwner: hasOwner, CreatedAt: f.CreatedAt,
			DayEnd:          firing.DayEnd(f.ScheduledDate, loc),
			DayReminderAt:   firing.DayStart(f.ScheduledDate, loc).Add(time.Duration(st.DayReminderHour) * time.Hour),
			RequestsCloseAt: f.RequestsCloseAt, ClosedManually: f.RequestsClosedManually,
			FiringEndsAt: f.FiringEndsAt, UnloadableAt: f.UnloadableAt, FullnessPct: full.Display,
			CloseHandledAt: f.RequestsCloseHandledAt, MissedStartSentAt: f.MissedStartReminderSentAt,
			MissedStartEscalatedAt:  f.MissedStartEscalatedAt,
			MissedUnloadSentAt:      f.MissedUnloadReminderSentAt,
			MissedUnloadEscalatedAt: f.MissedUnloadEscalatedAt,
			DayReminderSentAt:       f.DayReminderSentAt,
		}, rulesFrom(st), now)
		if !due.Any() {
			return nil
		}
		acted = true
		return r.apply(ctx, q, f, st, hasOwner, full.Display, due, now)
	})
	return acted, err
}

// hasResponsibleOwner is true only if the firing has an owner who is still a
// kiln master. A stale assignment is treated as unclaimed.
func (r *Runner) hasResponsibleOwner(ctx context.Context, q *db.Queries, f db.Firing) bool {
	if f.ManagedByUserID == nil {
		return false
	}
	ok, err := q.UserIsKilnMaster(ctx, *f.ManagedByUserID)
	return err == nil && ok
}

// responsible returns the DM recipients and the owner's name (empty when the
// firing is unclaimed).
func (r *Runner) responsible(ctx context.Context, q *db.Queries, f db.Firing) ([]int64, string, error) {
	if r.hasResponsibleOwner(ctx, q, f) {
		name, err := q.GetUserName(ctx, *f.ManagedByUserID)
		if err != nil {
			return nil, "", err
		}
		return []int64{*f.ManagedByUserID}, name, nil
	}
	ids, err := q.ListKilnMasterIDs(ctx)
	return ids, "", err
}

func (r *Runner) apply(ctx context.Context, q *db.Queries, f db.Firing, st db.Setting, hasOwner bool, pct int, due Result, now time.Time) error {
	loc := studioLoc(st.Timezone)
	url := notify.FiringURL(r.notifyCfg.BaseURL, f.ID)
	responsible, ownerName, err := r.responsible(ctx, q, f)
	if err != nil {
		return err
	}

	event := func(typ, note string) error {
		return q.InsertEvent(ctx, db.InsertEventParams{
			FiringID: f.ID, Type: typ, OccurredAt: now, Note: &note, Source: db.EventSourceSystem,
		})
	}
	var marks db.MarkJobsDoneParams
	marks.ID = f.ID
	batch := notify.Batch{SkipChannel: !(due.CloseRequests || due.AutoCooling)}

	if due.AutoCooling && f.FiringEndsAt != nil {
		coolingAt := *f.FiringEndsAt
		if _, err := q.AdvanceFiring(ctx, db.AdvanceFiringParams{
			ID: f.ID, Status: firing.Cooling, CoolingAt: &coolingAt,
		}); err != nil {
			return err
		}
		from, to := firing.Firing, firing.Cooling
		if err := q.InsertEvent(ctx, db.InsertEventParams{
			FiringID: f.ID, Type: firing.EventStatusChange, FromStatus: &from, ToStatus: &to,
			OccurredAt: coolingAt, Note: ptr("Cooling automatically after the profile's firing time"),
			Source: db.EventSourceSystem,
		}); err != nil {
			return err
		}
	}

	if due.CloseRequests {
		marks.RequestsCloseHandledAt = &now
		if !f.RequestsClosedManually {
			if err := event(firing.EventRequestsClosed, "Requests closed automatically at the cutoff"); err != nil {
				return err
			}
		}
		switch {
		case due.Underfilled:
			marks.UnderfilledAlertSentAt = &now
			batch.DM(responsible, notify.AlertUnderfilled(f, loc, pct, !hasOwner, url))
			if err := event(firing.EventAlertSent, "Kiln masters alerted: under-filled with requests closed"); err != nil {
				return err
			}
		case due.NeedsOwner:
			batch.DM(responsible, notify.AlertNeedsOwner(f, loc, pct, url))
			if err := event(firing.EventAlertSent, "Kiln masters alerted: needs a kiln master"); err != nil {
				return err
			}
		}
	}

	if due.MissedStart {
		marks.MissedStartReminderSentAt = &now
		batch.DM(responsible, notify.AlertMissedStart(f, loc, url))
		if err := event(firing.EventAlertSent, "Kiln masters reminded: not marked started"); err != nil {
			return err
		}
	}
	if due.MissedStartEscalation {
		marks.MissedStartEscalatedAt = &now
		ids, err := q.ListEscalationIDs(ctx)
		if err != nil {
			return err
		}
		batch.DM(ids, notify.AlertMissedStartEscalation(f, loc, ownerName, url))
		if err := event(firing.EventAlertSent, "Escalated: firing still not marked started"); err != nil {
			return err
		}
	}
	if due.MissedUnload {
		marks.MissedUnloadReminderSentAt = &now
		batch.DM(responsible, notify.AlertMissedUnload(f, loc, url))
		if err := event(firing.EventAlertSent, "Reminder sent: not marked unloaded"); err != nil {
			return err
		}
	}
	if due.MissedUnloadEscalation {
		marks.MissedUnloadEscalatedAt = &now
		ids, err := q.ListEscalationIDs(ctx)
		if err != nil {
			return err
		}
		batch.DM(ids, notify.AlertMissedUnloadEscalation(f, loc, ownerName, url))
		if err := event(firing.EventAlertSent, "Escalated: firing still not marked unloaded"); err != nil {
			return err
		}
	}
	if due.DayReminder {
		marks.DayReminderSentAt = &now
		batch.DM(responsible, notify.AlertMorning(f, loc, !hasOwner, url))
		if err := event(firing.EventAlertSent, "Kiln masters reminded: firing is today"); err != nil {
			return err
		}
	}

	if _, err := q.MarkJobsDone(ctx, marks); err != nil {
		return err
	}
	r.log.Info("jobs ran", "firing", f.ID, "auto_cooling", due.AutoCooling, "close", due.CloseRequests,
		"underfilled", due.Underfilled, "needs_owner", due.NeedsOwner, "missed_start", due.MissedStart,
		"missed_start_escalation", due.MissedStartEscalation, "missed_unload", due.MissedUnload,
		"missed_unload_escalation", due.MissedUnloadEscalation, "day_reminder", due.DayReminder)
	return notify.Enqueue(ctx, q, r.notifyCfg, f.ID, 0, &batch)
}

func ptr[T any](v T) *T { return &v }

// ── Membership re-check ─────────────────────────────────────────────────────

// cleanup deletes expired sessions and OAuth states. Starting a sign-in needs
// no login, so without this oauth_states would grow without bound.
func (r *Runner) cleanup(ctx context.Context) {
	q := db.New(r.pool)
	if err := q.DeleteExpiredSessions(ctx); err != nil {
		r.log.Error("deleting expired sessions failed", "error", err)
	}
	if err := q.DeleteExpiredOAuthStates(ctx); err != nil {
		r.log.Error("deleting expired OAuth states failed", "error", err)
	}
}

// RecheckMembers asks each provider about every account that has a live
// session or the kiln master flag. People who have left are signed out and
// lose the flag; a changed Kiln Master role is applied. When a provider can't
// answer (outage, missing permission) nothing changes for that account.
func (r *Runner) RecheckMembers(ctx context.Context) error {
	q := db.New(r.pool)
	accounts, err := q.ListAccountsToRecheck(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, a := range accounts {
		c := r.Checkers[a.Provider]
		if c == nil {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		m, err := c.CheckMember(ctx, a.ProviderUserID)
		if err != nil {
			r.log.Warn("membership check failed; leaving access unchanged", "account", a.ID, "provider", a.Provider, "error", err)
			errs = append(errs, err)
			continue
		}
		km := m.Member && m.KilnMaster
		if km != a.IsKilnMaster {
			if err := q.SetAccountKilnMaster(ctx, db.SetAccountKilnMasterParams{ID: a.ID, IsKilnMaster: km}); err != nil {
				errs = append(errs, err)
				continue
			}
			r.log.Info("kiln master role changed", "user", a.UserID, "provider", a.Provider, "kiln_master", km)
		}
		if !m.Member {
			n, err := q.DeleteUserSessions(ctx, a.UserID)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if n > 0 {
				r.log.Info("signed out: no longer a member", "user", a.UserID, "provider", a.Provider, "sessions", n)
			}
		}
	}
	return errors.Join(errs...)
}
