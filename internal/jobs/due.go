// Package jobs runs the scheduled checks from spec §10: closing requests at the
// cutoff, kiln-master alerts for under-filled or unconfirmed firings, and
// reminders for missed starts and unloads. Due is pure; Runner applies it.
package jobs

import (
	"slices"
	"time"

	"github.com/spencercornish/kilnlab/internal/firing"
)

// Snapshot is what the jobs need to know about one firing.
type Snapshot struct {
	Status            firing.Status
	CreatedAt         time.Time
	ScheduledStart    time.Time
	RequestsCloseAt   time.Time
	ClosedManually    bool
	EstimatedUnloadAt *time.Time
	FullnessPct       int // display fullness (override wins)

	CloseHandledAt     *time.Time
	MissedStartSentAt  *time.Time
	MissedUnloadSentAt *time.Time
}

// Rules are the settings that tune the jobs.
type Rules struct {
	UnderfilledPct    int
	MissedStartGrace  time.Duration
	MissedUnloadGrace time.Duration
}

// Result says what is due for a firing right now.
type Result struct {
	CloseRequests     bool // refresh the channel message ("Requests closed") and mark handled
	Underfilled       bool // DM kiln masters: under the threshold with requests closed
	NeedsConfirmation bool // DM kiln masters: requests closed but still proposed / on hold
	MissedStart       bool
	MissedUnload      bool
}

// Any reports whether anything is due.
func (r Result) Any() bool {
	return r.CloseRequests || r.Underfilled || r.NeedsConfirmation || r.MissedStart || r.MissedUnload
}

var (
	requestsStatuses = []firing.Status{firing.Proposed, firing.OnHold, firing.Confirmed}
	unconfirmed      = []firing.Status{firing.Proposed, firing.OnHold}
	beforeStart      = []firing.Status{firing.Confirmed, firing.Loading, firing.Loaded}
	inKiln           = []firing.Status{firing.Firing, firing.Cooling}
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
			out.NeedsConfirmation = slices.Contains(unconfirmed, s.Status)
		}
	}

	if slices.Contains(beforeStart, s.Status) && s.MissedStartSentAt == nil &&
		now.After(s.ScheduledStart.Add(r.MissedStartGrace)) {
		out.MissedStart = true
	}

	if slices.Contains(inKiln, s.Status) && s.MissedUnloadSentAt == nil && s.EstimatedUnloadAt != nil &&
		now.After(s.EstimatedUnloadAt.Add(r.MissedUnloadGrace)) {
		out.MissedUnload = true
	}
	return out
}
