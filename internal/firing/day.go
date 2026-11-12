package firing

import (
	"fmt"
	"time"
)

// ── Scheduled day (date-only contract) ──────────────────────────────────────
//
// A firing is booked for a calendar day in the studio's timezone. No start
// time is promised: the kiln runs sometime that day. Days are stored and
// passed around as a time.Time at UTC midnight whose date fields are the
// calendar date (what pgx returns for a DATE column). DayStart and DayEnd turn
// a day into the instants the window, notice and reminder rules compare with.

// DateLayout is the wire and storage form of a scheduled day.
const DateLayout = "2006-01-02"

// ParseDay reads a YYYY-MM-DD day.
func ParseDay(s string) (time.Time, error) {
	d, err := time.Parse(DateLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("date must look like 2026-11-20")
	}
	return d, nil
}

// DayOf is the calendar day (UTC midnight of that date) that t falls on in loc.
func DayOf(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// DayStart is the instant the day begins in loc. Rules compare against it: a
// firing "starts" at the beginning of its day for notice and the request cutoff.
func DayStart(day time.Time, loc *time.Location) time.Time {
	y, m, d := day.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

// DayEnd is the instant the day ends in loc (the next day's start). A firing
// that hasn't begun by then is overdue.
func DayEnd(day time.Time, loc *time.Location) time.Time {
	return DayStart(day.AddDate(0, 0, 1), loc)
}

// DayLabel is how a scheduled day reads to people, e.g. "Fri Nov 20".
func DayLabel(day time.Time) string { return day.Format("Mon Jan 2") }
