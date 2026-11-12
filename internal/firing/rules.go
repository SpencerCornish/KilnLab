package firing

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// ── Requests window (§6) ────────────────────────────────────────────────────

var requestsOpenStatuses = []Status{Proposed, OnHold, Confirmed}

// RequestsCloseAt = the scheduled day's start − cutoff (see DayStart).
func RequestsCloseAt(start time.Time, cutoffHours int32) time.Time {
	return start.Add(-time.Duration(cutoffHours) * time.Hour)
}

// Window is the part of a firing that decides whether requests are open.
type Window struct {
	Status         Status
	CloseAt        time.Time
	ClosedManually bool
}

// RequestsOpen: status ∈ {proposed, on_hold, confirmed} and now < close_at and
// not closed manually. Also gates editing/withdrawing your own request.
func RequestsOpen(w Window, now time.Time) bool {
	return slices.Contains(requestsOpenStatuses, w.Status) && now.Before(w.CloseAt) && !w.ClosedManually
}

// ReopenCloseAt is the close time to use when a kiln master reopens requests.
// Reopening after the automatic cutoff keeps requests open until the start.
func ReopenCloseAt(closeAt, start, now time.Time) time.Time {
	if now.Before(closeAt) {
		return closeAt
	}
	return start
}

// ── Minimum notice (§6) ─────────────────────────────────────────────────────

// CheckMinNotice returns an error if start is sooner than now + minNoticeHours.
// Callers skip it for users who may bypass (kiln masters).
func CheckMinNotice(start, now time.Time, minNoticeHours int32) error {
	earliest := now.Add(time.Duration(minNoticeHours) * time.Hour)
	if start.Before(earliest) {
		return fmt.Errorf("firings need at least %d hours' notice; ask a kiln master for anything sooner", minNoticeHours)
	}
	return nil
}

// ── Fullness (§7) ───────────────────────────────────────────────────────────

// Fullness summarises how full a firing is.
type Fullness struct {
	Calculated int  `json:"calculated_pct"` // proposer share + approved requests, uncapped
	Display    int  `json:"display_pct"`    // override if set, else calculated; capped at 100
	Overridden bool `json:"overridden"`     // display is set by a kiln master
	RoomLeft   int  `json:"room_left_pct"`  // 100 − display
}

// ComputeFullness: proposer share + every approved request; the override wins
// when set. Display values are capped to 0–100.
func ComputeFullness(proposerShare int32, approvedShares []int32, override *int32) Fullness {
	total := int(proposerShare)
	for _, s := range approvedShares {
		total += int(s)
	}
	f := Fullness{Calculated: total, Display: min(total, 100)}
	if override != nil {
		f.Display = max(0, min(int(*override), 100))
		f.Overridden = true
	}
	f.RoomLeft = 100 - f.Display
	return f
}

// WouldOverfill reports whether adding share to the calculated fullness goes
// over 100%. Allowed, but the requester is warned.
func WouldOverfill(calculated int, share int32) bool { return calculated+int(share) > 100 }

// ── Compatibility (§7) ──────────────────────────────────────────────────────

// Compatibility returns a warning if the work doesn't match the firing: a
// different cone, or glazed work going into a bisque firing. Empty means fine.
// It never blocks; the kiln master decides.
func Compatibility(firingType, firingCone, workCone string, glazed bool) string {
	var w []string
	if workCone != firingCone {
		w = append(w, fmt.Sprintf("Work is for cone %s but this is a cone %s firing.", workCone, firingCone))
	}
	if glazed && strings.EqualFold(firingType, "bisque") {
		w = append(w, "Glazed work in a bisque firing.")
	}
	return strings.Join(w, " ")
}
