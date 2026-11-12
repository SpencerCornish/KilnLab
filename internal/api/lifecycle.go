package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spencercornish/kilnlab/internal/db"
	"github.com/spencercornish/kilnlab/internal/firing"
	"github.com/spencercornish/kilnlab/internal/notify"
)

// ── Programs (settings.programs) ────────────────────────────────────────────

type programDTO struct {
	Name                   string `json:"name"`
	DefaultDurationMinutes *int32 `json:"default_duration_minutes,omitempty"`
}

func parsePrograms(raw json.RawMessage) []programDTO {
	var ps []programDTO
	if err := json.Unmarshal(raw, &ps); err != nil || ps == nil {
		return []programDTO{}
	}
	return ps
}

// ── Status changes (loading … completed) ────────────────────────────────────

const (
	maxProgram     = 100
	maxDurationMin = 7 * 24 * 60
)

type advanceInput struct {
	To                       db.FiringStatus `json:"to"`
	OccurredAt               *time.Time      `json:"occurred_at"` // back-dating; kiln masters only
	Program                  *string         `json:"program"`
	EstimatedDurationMinutes *int32          `json:"estimated_duration_minutes"`
	Note                     *string         `json:"note"`
	Source                   db.EventSource  `json:"source"` // web | kiln_qr
}

func timestampsOf(f db.Firing) firing.Timestamps {
	return firing.Timestamps{StartedAt: f.StartedAt, CoolingAt: f.CoolingAt, UnloadedAt: f.UnloadedAt,
		ReadyForPickupAt: f.ReadyForPickupAt, CompletedAt: f.CompletedAt}
}

// occurredAt resolves when a change happened: now, or a back-dated time for
// users allowed to correct timestamps.
func (s *Server) occurredAt(u *currentUser, at *time.Time) (time.Time, error) {
	now := s.now()
	if at == nil {
		return now, nil
	}
	t := at.UTC()
	if !u.Roles.CanCorrectTimestamps() {
		if d := t.Sub(now); d > firing.ClockSkew || d < -firing.ClockSkew {
			return now, forbidden("only kiln masters can set a different time")
		}
		return now, nil
	}
	if err := firing.CheckOccurredAt(t, now); err != nil {
		return now, badRequest("%s", strings.TrimPrefix(err.Error(), firing.ErrBadTime.Error()+": "))
	}
	return t, nil
}

func (s *Server) handleAdvance(w http.ResponseWriter, r *http.Request) {
	var in advanceInput
	if !s.readJSON(w, r, &in) {
		return
	}
	s.mutateFiring(w, r, func(ctx context.Context, q *db.Queries, f db.Firing, u *currentUser, n *notify.Batch) ([]string, error) {
		if !in.To.Valid() {
			return nil, badRequest("unknown status")
		}
		if err := firing.Advance(stateOf(f), in.To, actorFor(u)); err != nil {
			return nil, err
		}
		source := db.EventSourceWeb
		if in.Source == db.EventSourceKilnQr {
			source = in.Source
		}
		at, err := s.occurredAt(u, in.OccurredAt)
		if err != nil {
			return nil, err
		}
		ts := timestampsOf(f)
		field := firing.TimestampField(in.To)
		if latest := ts.Latest(); latest != nil && at.Before(*latest) {
			return nil, badRequest("that time is before the firing's last recorded step")
		}
		if field != "" {
			if err := ts.With(field, at).CheckOrder(); err != nil {
				return nil, badRequest("%s", err.Error())
			}
		}
		note, err := optText("note", in.Note, maxReason)
		if err != nil {
			return nil, err
		}

		p := db.AdvanceFiringParams{ID: f.ID, Status: in.To}
		var eventNote *string
		switch in.To {
		case firing.Firing:
			program, err := text("program", ptrOr(in.Program, ""), maxProgram, true)
			if err != nil {
				return nil, err
			}
			if in.EstimatedDurationMinutes == nil || *in.EstimatedDurationMinutes < 1 || *in.EstimatedDurationMinutes > maxDurationMin {
				return nil, badRequest("estimated duration is required (in minutes, up to 7 days)")
			}
			unload := firing.EstimatedUnload(at, *in.EstimatedDurationMinutes)
			p.Program, p.EstimatedDurationMinutes, p.StartedAt, p.EstimatedUnloadAt = &program, in.EstimatedDurationMinutes, &at, &unload
			n := fmt.Sprintf("%s, about %s", program, formatDuration(*in.EstimatedDurationMinutes))
			if note != nil {
				n += ". " + *note
			}
			eventNote = &n
		case firing.Cooling:
			p.CoolingAt = &at
		case firing.Completed:
			// Unloaded, ready for pickup and completed are one step.
			p.UnloadedAt, p.ReadyForPickupAt, p.CompletedAt = &at, &at, &at
		}
		if eventNote == nil {
			eventNote = note
		}
		updated, err := q.AdvanceFiring(ctx, p)
		if err != nil {
			return nil, err
		}
		loc := s.studioLocation(ctx, q)
		switch in.To {
		case firing.Firing:
			n.ThreadReply(notify.ThreadStarted(updated, loc))
		case firing.Completed:
			n.ThreadReply(notify.ThreadReady())
			ids, err := q.ListParticipants(ctx, db.ListParticipantsParams{FiringID: f.ID, Statuses: []string{string(db.RequestStatusApproved)}})
			if err != nil {
				return nil, err
			}
			n.DM(ids, notify.DMReady(f, loc, notify.FiringURL(s.cfg.AppBaseURL, f.ID)))
		}
		from := f.Status
		return nil, q.InsertEvent(ctx, db.InsertEventParams{
			FiringID: f.ID, ActorUserID: &u.ID, Type: firing.EventStatusChange, FromStatus: &from, ToStatus: &in.To,
			OccurredAt: at, Note: eventNote, Source: source,
		})
	})
}

func ptrOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

func formatDuration(min int32) string {
	h, m := min/60, min%60
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dh %dm", h, m)
	}
}

// ── Correcting timestamps after the fact ────────────────────────────────────

type correctInput struct {
	Field string    `json:"field"` // started_at | cooling_at | unloaded_at
	At    time.Time `json:"at"`
}

func (s *Server) handleCorrectTimestamp(w http.ResponseWriter, r *http.Request) {
	var in correctInput
	if !s.readJSON(w, r, &in) {
		return
	}
	s.mutateFiring(w, r, func(ctx context.Context, q *db.Queries, f db.Firing, u *currentUser, n *notify.Batch) ([]string, error) {
		if !u.Roles.CanCorrectTimestamps() {
			return nil, forbidden("only kiln masters can correct times")
		}
		if !slices.Contains(firing.CorrectableFields, in.Field) {
			return nil, badRequest("unknown time field")
		}
		ts := timestampsOf(f)
		old := ts.Get(in.Field)
		if old == nil {
			return nil, conflict("that step hasn't been recorded yet")
		}
		at := in.At.UTC()
		if at.After(s.now().Add(firing.ClockSkew)) {
			return nil, badRequest("that time is in the future")
		}
		ts = ts.With(in.Field, at)
		if err := ts.CheckOrder(); err != nil {
			return nil, badRequest("%s", err.Error())
		}
		unload := f.EstimatedUnloadAt
		if in.Field == "started_at" && f.EstimatedDurationMinutes != nil {
			u := firing.EstimatedUnload(at, *f.EstimatedDurationMinutes)
			unload = &u
		}
		if _, err := q.CorrectTimestamps(ctx, db.CorrectTimestampsParams{
			ID: f.ID, StartedAt: ts.StartedAt, EstimatedUnloadAt: unload, CoolingAt: ts.CoolingAt,
			UnloadedAt: ts.UnloadedAt, ReadyForPickupAt: ts.ReadyForPickupAt, CompletedAt: ts.CompletedAt,
		}); err != nil {
			return nil, err
		}
		loc := s.studioLocation(ctx, q)
		const layout = "Mon Jan 2, 3:04 PM"
		note := fmt.Sprintf("Corrected %s time from %s to %s", fieldNames[in.Field],
			old.In(loc).Format(layout), at.In(loc).Format(layout))
		return nil, s.logEvent(ctx, q, f, u, firing.EventTimestampCorrected, nil, nil, &note)
	})
}

var fieldNames = map[string]string{
	"started_at": "start", "cooling_at": "cooling", "unloaded_at": "unloaded",
}

func (s *Server) studioLocation(ctx context.Context, q *db.Queries) *time.Location {
	st, err := q.GetSettings(ctx)
	if err != nil {
		return time.UTC
	}
	loc, err := time.LoadLocation(st.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// ── Kiln QR (§9B) ───────────────────────────────────────────────────────────

type kilnFiringDTO struct {
	ID            int64           `json:"id"`
	KilnID        int64           `json:"kiln_id"`
	Title         *string         `json:"title"`
	FiringType    string          `json:"firing_type"`
	Cone          string          `json:"cone"`
	ScheduledDate Day             `json:"scheduled_date"`
	Status        db.FiringStatus `json:"status"`
	ProposerName  string          `json:"proposer_name"`
}

type kilnViewDTO struct {
	Kiln    *kilnDTO        `json:"kiln"` // nil when no ?k= was given
	Firings []kilnFiringDTO `json:"firings"`
}

// handleKiln lists the firings relevant at the kiln right now: in progress, or
// confirmed and starting within ±24h. ?k=<kiln id> narrows to one kiln (each
// kiln's QR code carries its id).
func (s *Server) handleKiln(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var out kilnViewDTO
	var kilnID int64
	if k := r.URL.Query().Get("k"); k != "" {
		id, err := strconv.ParseInt(k, 10, 64)
		if err != nil {
			s.fail(w, r, errNotFound)
			return
		}
		kiln, err := s.queries.GetKiln(ctx, id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		dto := toKilnDTO(kiln)
		out.Kiln, kilnID = &dto, id
	}
	now := s.now()
	loc := s.studioLocation(ctx, s.queries)
	rows, err := s.queries.ListKilnCandidates(ctx, db.ListKilnCandidatesParams{
		KilnID: kilnID, WindowStart: firing.DayOf(now.Add(-firing.KilnRelevanceWindow-24*time.Hour), loc),
		WindowEnd: firing.DayOf(now.Add(firing.KilnRelevanceWindow+24*time.Hour), loc),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out.Firings = []kilnFiringDTO{}
	for _, row := range rows {
		f := row.Firing
		if !firing.RelevantAtKiln(f.Status, firing.DayStart(f.ScheduledDate, loc), now) {
			continue
		}
		out.Firings = append(out.Firings, kilnFiringDTO{ID: f.ID, KilnID: f.KilnID, Title: f.Title,
			FiringType: f.FiringType, Cone: f.Cone, ScheduledDate: Day(f.ScheduledDate), Status: f.Status,
			ProposerName: row.ProposerName})
	}
	writeJSON(w, http.StatusOK, out)
}

// ── People (/admin/users) ───────────────────────────────────────────────────

type adminUserDTO struct {
	ID           int64           `json:"id"`
	DisplayName  string          `json:"display_name"`
	AvatarURL    *string         `json:"avatar_url"`
	Community    db.Community    `json:"community"`
	Provider     db.AuthProvider `json:"provider"`
	IsKilnMaster bool            `json:"is_kiln_master"`
	IsAdmin      bool            `json:"is_admin"`
	LastLoginAt  time.Time       `json:"last_login_at"`
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if !userFromContext(r.Context()).Roles.CanEditSettings() {
		s.fail(w, r, forbidden("only admins can manage people"))
		return
	}
	rows, err := s.queries.ListUsers(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]adminUserDTO, 0, len(rows))
	for _, u := range rows {
		out = append(out, adminUserDTO{ID: u.ID, DisplayName: u.DisplayName, AvatarURL: u.AvatarUrl,
			Community: u.Community, Provider: u.Provider,
			IsKilnMaster: u.IsKilnMaster, IsAdmin: u.IsAdmin, LastLoginAt: u.LastLoginAt})
	}
	writeJSON(w, http.StatusOK, out)
}

type updateUserInput struct {
	Admin *bool `json:"admin"`
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	var in updateUserInput
	if !s.readJSON(w, r, &in) {
		return
	}
	me := userFromContext(r.Context())
	err := func() error {
		id, err := idParam(r, "id")
		if err != nil {
			return err
		}
		if in.Admin != nil {
			if !me.Roles.CanEditSettings() {
				return forbidden("only admins can change admins")
			}
			if id == me.ID && !*in.Admin {
				return conflict("you can't remove your own admin access")
			}
		}
		return db.InTx(r.Context(), s.pool, func(q *db.Queries) error {
			if _, err := q.GetUser(r.Context(), id); err != nil {
				return err
			}
			if in.Admin != nil {
				if _, err := q.SetUserAdmin(r.Context(), db.SetUserAdminParams{ID: id, IsAdmin: *in.Admin}); err != nil {
					return err
				}
			}
			return nil
		})
	}()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Notification log (/admin) ───────────────────────────────────────────────

type outboxDTO struct {
	ID            int64           `json:"id"`
	Kind          db.OutboxKind   `json:"kind"`
	Platform      db.AuthProvider `json:"platform"`
	Status        db.OutboxStatus `json:"status"`
	Attempts      int32           `json:"attempts"`
	LastError     *string         `json:"last_error"`
	NextAttemptAt time.Time       `json:"next_attempt_at"`
	CreatedAt     time.Time       `json:"created_at"`
	Payload       json.RawMessage `json:"payload"`
}

// handleListOutbox shows recent notifications so admins can spot failures.
func (s *Server) handleListOutbox(w http.ResponseWriter, r *http.Request) {
	if !userFromContext(r.Context()).Roles.CanEditSettings() {
		s.fail(w, r, forbidden("only admins can see the notification log"))
		return
	}
	rows, err := s.queries.ListOutbox(r.Context(), 100)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]outboxDTO, 0, len(rows))
	for _, o := range rows {
		out = append(out, outboxDTO{ID: o.ID, Kind: o.Kind, Platform: o.Platform, Status: o.Status, Attempts: o.Attempts,
			LastError: o.LastError, NextAttemptAt: o.NextAttemptAt, CreatedAt: o.CreatedAt, Payload: o.Payload})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"dry_run": s.notifyCfg.DryRun, "discord": s.notifyCfg.DiscordBotToken != "", "slack": s.notifyCfg.SlackBotToken != "",
		"items": out,
	})
}
