// Package jobs runs the scheduled checks from spec §10: auto-cooling at the
// profile's firing end, closing requests at the cutoff, kiln-master alerts for
// under-filled or unclaimed firings, day-of reminders, and missed
// start/unload reminders with escalation. Due is pure; Runner applies it.
package jobs

import (
	"slices"
	"time"

	"github.com/spencercornish/kilnlab/internal/firing"
)

// Snapshot is what the jobs need to know about one firing.
type Snapshot struct {
	Status          firing.Status
	HasOwner        bool
	CreatedAt       time.Time
	DayEnd          time.Time // when the scheduled day ends: a firing not begun by then is missed
	DayReminderAt   time.Time // when the day-of reminder is due (day start + setting, studio time); zero = none
	RequestsCloseAt time.Time
	ClosedManually  bool
	FiringEndsAt    *time.Time
	UnloadableAt    *time.Time
	FullnessPct     int // display fullness (override wins)

	CloseHandledAt          *time.Time
	MissedStartSentAt       *time.Time
	MissedStartEscalatedAt  *time.Time
	MissedUnloadSentAt      *time.Time
	MissedUnloadEscalatedAt *time.Time
	DayReminderSentAt       *time.Time
}

// Rules are the settings that tune the jobs.
type Rules struct {
	UnderfilledPct    int
	MissedStartGrace  time.Duration
	MissedUnloadGrace time.Duration
	EscalationGrace   time.Duration
}

// Result says what is due for a firing right now.
type Result struct {
	CloseRequests          bool // refresh the channel message ("Requests closed") and mark handled
	Underfilled            bool // alert: under the threshold with requests closed
	NeedsOwner             bool // alert: no kiln master has taken the firing
	MissedStart            bool
	MissedStartEscalation  bool
	MissedUnload           bool
	MissedUnloadEscalation bool
	DayReminder            bool // DM responsible KMs: the firing is today
	AutoCooling            bool // firing → cooling at the profile's firing end
}

// Any reports whether anything is due.
func (r Result) Any() bool {
	return r.CloseRequests || r.Underfilled || r.NeedsOwner || r.MissedStart ||
		r.MissedStartEscalation || r.MissedUnload || r.MissedUnloadEscalation ||
		r.DayReminder || r.AutoCooling
}

var (
	requestsStatuses    = []firing.Status{firing.Proposed, firing.OnHold, firing.Scheduled}
	inKiln              = []firing.Status{firing.Firing, firing.Cooling}
	dayReminderStatuses = []firing.Status{firing.Proposed, firing.OnHold, firing.Scheduled}
)

// Due decides which jobs apply to s at now. Each job runs once: the caller
// records the matching *_sent_at / handled marker, and Due skips set markers.
func Due(s Snapshot, r Rules, now time.Time) Result {
	var out Result

	closed := s.ClosedManually || !now.Before(s.RequestsCloseAt)
	if slices.Contains(requestsStatuses, s.Status) && closed && s.CloseHandledAt == nil {
		out.CloseRequests = true
		// A firing created after its own cutoff (a kiln master scheduling at
		// short notice) was never open for requests; don't alert about it.
		if s.CreatedAt.Before(s.RequestsCloseAt) || s.ClosedManually {
			out.Underfilled = s.FullnessPct < r.UnderfilledPct
			out.NeedsOwner = !s.HasOwner
		}
	}

	if s.Status == firing.Scheduled && s.MissedStartSentAt == nil &&
		now.After(s.DayEnd.Add(r.MissedStartGrace)) {
		out.MissedStart = true
	}
	if s.Status == firing.Scheduled && s.HasOwner && s.MissedStartSentAt != nil &&
		s.MissedStartEscalatedAt == nil && now.After(s.MissedStartSentAt.Add(r.EscalationGrace)) {
		out.MissedStartEscalation = true
	}

	if slices.Contains(inKiln, s.Status) && s.MissedUnloadSentAt == nil && s.UnloadableAt != nil &&
		now.After(s.UnloadableAt.Add(r.MissedUnloadGrace)) {
		out.MissedUnload = true
	}
	if slices.Contains(inKiln, s.Status) && s.HasOwner && s.MissedUnloadSentAt != nil &&
		s.MissedUnloadEscalatedAt == nil && now.After(s.MissedUnloadSentAt.Add(r.EscalationGrace)) {
		out.MissedUnloadEscalation = true
	}

	// Day-of reminder, once, on the morning of the firing's day. A firing created
	// after its own reminder instant (scheduled at short notice that morning)
	// already knows it's today, so it isn't reminded.
	if !s.DayReminderAt.IsZero() && s.DayReminderSentAt == nil &&
		slices.Contains(dayReminderStatuses, s.Status) && s.CreatedAt.Before(s.DayReminderAt) &&
		!now.Before(s.DayReminderAt) {
		out.DayReminder = true
	}

	if s.Status == firing.Firing && s.FiringEndsAt != nil && !now.Before(*s.FiringEndsAt) {
		out.AutoCooling = true
	}
	return out
}
