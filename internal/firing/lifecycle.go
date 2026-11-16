package firing

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// ── Next actions (§9B step 3) ───────────────────────────────────────────────

// NextSteps are the user-driven lifecycle statuses to offer this actor: START
// FIRING for kiln masters on a scheduled firing, and UNLOADED for anyone once
// the kiln has started. Cooling is automatic, so it is never offered.
func NextSteps(s State, a Actor) []Status {
	switch s.Status {
	case Scheduled:
		if a.Roles.CanStartFiring() {
			return []Status{Firing}
		}
	case Firing, Cooling:
		return []Status{Completed}
	}
	return nil
}

// BlockedReason explains, for an actor with no next steps, why not. Empty when
// there is nothing to explain.
func BlockedReason(s State, a Actor) string {
	if len(NextSteps(s, a)) > 0 {
		return ""
	}
	if s.Status == Scheduled {
		return "Only kiln masters can start the kiln."
	}
	return ""
}

// ── Relevant firings for the kiln QR (§9B step 2) ───────────────────────────

var inProgress = []Status{Firing, Cooling}

// activeAtKiln are the pre-start statuses that can be claimed/started at the wall.
var activeAtKiln = []Status{Proposed, OnHold, Scheduled}

// KilnRelevanceWindow: pre-start firings this close to their day count.
const KilnRelevanceWindow = 24 * time.Hour

// RelevantAtKiln: any in-progress firing, or a proposed/on-hold/scheduled one
// within ±24h of its day. Unclaimed firings appear so a KM can claim them.
func RelevantAtKiln(status Status, start, now time.Time) bool {
	if slices.Contains(inProgress, status) {
		return true
	}
	if !slices.Contains(activeAtKiln, status) {
		return false
	}
	d := start.Sub(now)
	return d <= KilnRelevanceWindow && d >= -KilnRelevanceWindow
}

// ── Timestamps & back-dating (§6, §12) ──────────────────────────────────────

// Timestamps are a firing's recorded lifecycle times, in order. FiringEndsAt
// and UnloadableAt are estimates derived from the profile, not recorded events.
type Timestamps struct {
	StartedAt, CoolingAt, UnloadedAt *time.Time
	FiringEndsAt, UnloadableAt       *time.Time
}

// TimestampField is the firings column a status sets, or "" for statuses that
// only write an event.
func TimestampField(to Status) string {
	switch to {
	case Firing:
		return "started_at"
	case Cooling:
		return "cooling_at"
	case Completed:
		return "unloaded_at"
	}
	return ""
}

// CorrectableFields in lifecycle order.
var CorrectableFields = []string{"started_at", "cooling_at", "unloaded_at"}

var timestampNames = []string{"started_at", "cooling_at", "unloaded_at"}

func (t Timestamps) recorded() []*time.Time {
	return []*time.Time{t.StartedAt, t.CoolingAt, t.UnloadedAt}
}

// Get returns the timestamp for a correctable field name.
func (t Timestamps) Get(field string) *time.Time {
	if !slices.Contains(CorrectableFields, field) {
		return nil
	}
	return t.recorded()[slices.Index(timestampNames, field)]
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
	}
	return t
}

// Latest is the most recent recorded timestamp, or nil.
func (t Timestamps) Latest() *time.Time {
	var latest *time.Time
	for _, ts := range t.recorded() {
		if ts != nil && (latest == nil || ts.After(*latest)) {
			latest = ts
		}
	}
	return latest
}

// CheckOrder: recorded timestamps must not go backwards (cooling before
// started, unloaded before cooling, etc.).
func (t Timestamps) CheckOrder() error {
	var prev *time.Time
	prevName := ""
	for i, ts := range t.recorded() {
		if ts == nil {
			continue
		}
		if prev != nil && ts.Before(*prev) {
			return fmt.Errorf("%s can't be before %s", fieldLabel(timestampNames[i]), fieldLabel(prevName))
		}
		prev, prevName = ts, timestampNames[i]
	}
	return nil
}

func fieldLabel(f string) string {
	return map[string]string{
		"started_at": "started", "cooling_at": "cooling", "unloaded_at": "unloaded",
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

// PlannedTimings derives the estimated firing end and unloadable moment from
// an actual start and the profile's rough times.
func PlannedTimings(start time.Time, firingMinutes, cooldownMinutes int32) (firingEnds, unloadable time.Time) {
	firingEnds = start.Add(time.Duration(firingMinutes) * time.Minute)
	unloadable = firingEnds.Add(time.Duration(cooldownMinutes) * time.Minute)
	return firingEnds, unloadable
}

// ReadyToUnload reports whether the rough cooldown has elapsed.
func ReadyToUnload(status Status, unloadableAt *time.Time, now time.Time) bool {
	if status != Firing && status != Cooling {
		return false
	}
	return unloadableAt != nil && !now.Before(*unloadableAt)
}
