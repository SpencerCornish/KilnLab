package firing

import (
	"testing"
	"time"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestBlackoutForDay(t *testing.T) {
	ranges := []Blackout{
		{Start: day(2026, 11, 20), End: day(2026, 11, 22), Reason: "elements"},
		{Start: day(2026, 12, 24), End: day(2026, 12, 26)},
	}
	cases := []struct {
		name   string
		d      time.Time
		want   bool
		reason string
	}{
		{"day before", day(2026, 11, 19), false, ""},
		{"first day", day(2026, 11, 20), true, "elements"},
		{"middle day", day(2026, 11, 21), true, "elements"},
		{"last day", day(2026, 11, 22), true, "elements"},
		{"day after", day(2026, 11, 23), false, ""},
		{"single other range", day(2026, 12, 25), true, ""},
		{"far away", day(2026, 12, 27), false, ""},
	}
	for _, c := range cases {
		got, ok := BlackoutForDay(c.d, ranges)
		if ok != c.want || got.Reason != c.reason {
			t.Errorf("%s: got (%+v, %v), want (%q, %v)", c.name, got, ok, c.reason, c.want)
		}
	}

	// The time of day is ignored: a firing day always matches by calendar date.
	withTime := day(2026, 11, 21).Add(17*time.Hour + 30*time.Minute)
	if _, ok := BlackoutForDay(withTime, ranges); !ok {
		t.Error("time of day should not matter")
	}

	if _, ok := BlackoutForDay(day(2026, 11, 21), nil); ok {
		t.Error("no ranges means never blocked")
	}
}
