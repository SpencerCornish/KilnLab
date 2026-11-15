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
		due := Due(Snapshot{
			Status: f.Status, CreatedAt: f.CreatedAt, DayEnd: firing.DayEnd(f.ScheduledDate, studioLoc(st.Timezone)),
			RequestsCloseAt: f.RequestsCloseAt, ClosedManually: f.RequestsClosedManually,
			EstimatedUnloadAt: f.EstimatedUnloadAt, FullnessPct: full.Display,
			CloseHandledAt: f.RequestsCloseHandledAt, MissedStartSentAt: f.MissedStartReminderSentAt,
			MissedUnloadSentAt: f.MissedUnloadReminderSentAt,
		}, rulesFrom(st), now)
		if !due.Any() {
			return nil
		}
		acted = true
		return r.apply(ctx, q, f, st, full.Display, due, now)
	})
	return acted, err
}

func (r *Runner) apply(ctx context.Context, q *db.Queries, f db.Firing, st db.Setting, pct int, due Result, now time.Time) error {
	loc := time.UTC
	if l, err := time.LoadLocation(st.Timezone); err == nil {
		loc = l
	}
	url := notify.FiringURL(r.notifyCfg.BaseURL, f.ID)
	kms, err := q.ListKilnMasterIDs(ctx)
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
	batch := notify.Batch{SkipChannel: !due.CloseRequests}

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
			batch.DM(kms, notify.AlertUnderfilled(f, loc, pct, due.NeedsConfirmation, url))
			if err := event(firing.EventAlertSent, "Kiln masters alerted: under-filled with requests closed"); err != nil {
				return err
			}
		case due.NeedsConfirmation:
			batch.DM(kms, notify.AlertNeedsConfirmation(f, loc, pct, url))
			if err := event(firing.EventAlertSent, "Kiln masters reminded: needs confirming"); err != nil {
				return err
			}
		}
	}
	if due.MissedStart {
		marks.MissedStartReminderSentAt = &now
		batch.DM(kms, notify.AlertMissedStart(f, loc, url))
		if err := event(firing.EventAlertSent, "Kiln masters reminded: not marked started"); err != nil {
			return err
		}
	}
	if due.MissedUnload {
		marks.MissedUnloadReminderSentAt = &now
		batch.DM(kms, notify.AlertMissedUnload(f, loc, url))
		if err := event(firing.EventAlertSent, "Reminder sent: not marked unloaded"); err != nil {
			return err
		}
	}

	if _, err := q.MarkJobsDone(ctx, marks); err != nil {
		return err
	}
	r.log.Info("jobs ran", "firing", f.ID, "close", due.CloseRequests, "underfilled", due.Underfilled,
		"needs_confirmation", due.NeedsConfirmation, "missed_start", due.MissedStart, "missed_unload", due.MissedUnload)
	return notify.Enqueue(ctx, q, r.notifyCfg, f.ID, 0, &batch)
}

// cleanup deletes expired sessions and OAuth states. Starting a sign-in needs
// no login, so without this oauth_states would grow without bound.
func (r *Runner) cleanup(ctx context.Context) {
	q := db.New(r.pool)
	if err := q.DeleteExpiredSessions(ctx); err != nil {
		r.log.Error("deleting expired sessions failed", "error", err)
	}
	if err := q.DeleteExpiredOAuthStates(ctx); err != nil {
		r.log.Error("deleting expired oauth states failed", "error", err)
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
