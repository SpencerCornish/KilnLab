package firing

import "time"

// Blackout is a closed range of calendar days a kiln can't take new firings.
// Start and End are inclusive days (DATE values: UTC midnight).
type Blackout struct {
	Start  time.Time
	End    time.Time
	Reason string
}

// BlackoutForDay returns the first blackout that covers day, if any. Days are
// compared as calendar dates, so the time of day doesn't matter.
func BlackoutForDay(day time.Time, ranges []Blackout) (Blackout, bool) {
	d := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	for _, b := range ranges {
		start := time.Date(b.Start.Year(), b.Start.Month(), b.Start.Day(), 0, 0, 0, 0, time.UTC)
		end := time.Date(b.End.Year(), b.End.Month(), b.End.Day(), 0, 0, 0, 0, time.UTC)
		if !d.Before(start) && !d.After(end) {
			return b, true
		}
	}
	return Blackout{}, false
}
