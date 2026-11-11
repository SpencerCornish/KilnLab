package firing

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// ── Next actions (§9B step 3) ───────────────────────────────────────────────

// NextSteps are the lifecycle statuses to offer this actor as buttons: the
// first step forward they're allowed to take, plus START FIRING for kiln
// masters while the kiln hasn't started yet (skipping is allowed).
func NextSteps(s State, a Actor) []Status {
	from := slices.Index(lifecycle, s.Status)
	if from < 0 {
		return nil
	}
	var out []Status
	for _, to := range lifecycle[from+1:] {
		if Advance(s, to, a) == nil {
			out = append(out, to)
			break
		}
	}
	if a.Roles.CanStartFiring() && from < slices.Index(lifecycle, Firing) && !slices.Contains(out, Firing) {
		out = append(out, Firing)
	}
	return out
}

// BlockedReason explains, for an actor with no next steps, why not. Empty
// when there's nothing to explain (e.g. the firing is finished).
func BlockedReason(s State, a Actor) string {
	if len(NextSteps(s, a)) > 0 || !slices.Contains(lifecycle[:len(lifecycle)-1], s.Status) {
		return ""
	}
	if s.Status == Loaded || (a.IsLoader && a.Roles.SelfLoader && s.Status == Loading) {
		return "Only kiln masters can start the kiln."
	}
	if a.IsLoader && a.Roles.SelfLoader {
		return "A kiln master needs to do the next step."
	}
	return "Only kiln masters and the assigned loader can update this firing."
}

// ── Relevant firings for the kiln QR (§9B step 2) ───────────────────────────

var inProgress = []Status{Loading, Loaded, Firing, Cooling, Unloaded}

// KilnRelevanceWindow: confirmed firings this close to their start count.
const KilnRelevanceWindow = 24 * time.Hour

// RelevantAtKiln: any in-progress firing, or a confirmed one starting within ±24h.
func RelevantAtKiln(status Status, start, now time.Time) bool {
	if slices.Contains(inProgress, status) {
		return true
	}
	if status != Confirmed {
		return false
	}
	d := start.Sub(now)
	return d <= KilnRelevanceWindow && d >= -KilnRelevanceWindow
}

// ── Timestamps & back-dating (§6, §12) ──────────────────────────────────────

// Timestamps are a firing's lifecycle times, in order.
type Timestamps struct {
	StartedAt, CoolingAt, UnloadedAt, ReadyForPickupAt, CompletedAt *time.Time
}

// TimestampField is the firings column a status sets, or "" for statuses
// that only write an event (loading, loaded).
func TimestampField(to Status) string {
	switch to {
	case Firing:
		return "started_at"
	case Cooling:
		return "cooling_at"
	case Unloaded:
		return "unloaded_at"
	case ReadyForPickup:
		return "ready_for_pickup_at"
	case Completed:
		return "completed_at"
	}
	return ""
}

// CorrectableFields in lifecycle order.
var CorrectableFields = []string{"started_at", "cooling_at", "unloaded_at", "ready_for_pickup_at", "completed_at"}

func (t Timestamps) ordered() []*time.Time {
	return []*time.Time{t.StartedAt, t.CoolingAt, t.UnloadedAt, t.ReadyForPickupAt, t.CompletedAt}
}

// Get returns the timestamp for a field name.
func (t Timestamps) Get(field string) *time.Time {
	if i := slices.Index(CorrectableFields, field); i >= 0 {
		return t.ordered()[i]
	}
	return nil
}

// With returns a copy with field set to at.
func (t Timestamps) With(field string, at time.Time) Timestamps {
	switch field {
	case "started_at":
		t.StartedAt = &at
	case "cooling_at":
		t.CoolingAt = &at
	case "unloaded_at":
		t.UnloadedAt = &at
	case "ready_for_pickup_at":
		t.ReadyForPickupAt = &at
	case "completed_at":
		t.CompletedAt = &at
	}
	return t
}

// Latest is the most recent set timestamp, or nil.
func (t Timestamps) Latest() *time.Time {
	var latest *time.Time
	for _, ts := range t.ordered() {
		if ts != nil && (latest == nil || ts.After(*latest)) {
			latest = ts
		}
	}
	return latest
}

// CheckOrder: set timestamps must not go backwards (cooling before started etc.).
func (t Timestamps) CheckOrder() error {
	var prev *time.Time
	prevName := ""
	for i, ts := range t.ordered() {
		if ts == nil {
			continue
		}
		if prev != nil && ts.Before(*prev) {
			return fmt.Errorf("%s can't be before %s", fieldLabel(CorrectableFields[i]), fieldLabel(prevName))
		}
		prev, prevName = ts, CorrectableFields[i]
	}
	return nil
}

func fieldLabel(f string) string {
	return map[string]string{
		"started_at": "started", "cooling_at": "cooling", "unloaded_at": "unloaded",
		"ready_for_pickup_at": "ready for pickup", "completed_at": "completed",
	}[f]
}

const (
	// ClockSkew tolerated for "now" times sent by phones.
	ClockSkew = 5 * time.Minute
	// MaxBackdate: how far back a kiln master may date a change.
	MaxBackdate = 7 * 24 * time.Hour
)

var ErrBadTime = errors.New("invalid time")

// CheckOccurredAt validates a (possibly back-dated) time for a change.
func CheckOccurredAt(at, now time.Time) error {
	if at.After(now.Add(ClockSkew)) {
		return fmt.Errorf("%w: that time is in the future", ErrBadTime)
	}
	if at.Before(now.Add(-MaxBackdate)) {
		return fmt.Errorf("%w: can't date a change more than 7 days back", ErrBadTime)
	}
	return nil
}

// EstimatedUnload = start + duration.
func EstimatedUnload(start time.Time, durationMinutes int32) time.Time {
	return start.Add(time.Duration(durationMinutes) * time.Minute)
}
