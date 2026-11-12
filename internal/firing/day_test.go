package firing

import (
	"testing"
	"time"
)

func TestDayBoundaries(t *testing.T) {
	denver, _ := time.LoadLocation("America/Denver")
	day := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC) // the calendar day Fri Nov 20

	if got, want := DayStart(day, denver), time.Date(2026, 11, 20, 7, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("DayStart = %v, want %v (local midnight)", got, want)
	}
	if got, want := DayEnd(day, denver), time.Date(2026, 11, 21, 7, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("DayEnd = %v, want %v", got, want)
	}
	// DST ends Sun Nov 1 2026: that day is 25 hours long in Denver.
	dst := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	if got := DayEnd(dst, denver).Sub(DayStart(dst, denver)); got != 25*time.Hour {
		t.Errorf("DST day length = %v", got)
	}
}

func TestDayOfUsesStudioDate(t *testing.T) {
	denver, _ := time.LoadLocation("America/Denver")
	// 9pm Thu Nov 19 in Denver is already Fri Nov 20 in UTC.
	at := time.Date(2026, 11, 20, 4, 0, 0, 0, time.UTC)
	if got := DayOf(at, denver); got.Format(DateLayout) != "2026-11-19" {
		t.Errorf("DayOf = %s, want 2026-11-19", got.Format(DateLayout))
	}
	if got := DayOf(at, time.UTC); got.Format(DateLayout) != "2026-11-20" {
		t.Errorf("DayOf in UTC = %s", got.Format(DateLayout))
	}
}

func TestParseDay(t *testing.T) {
	if d, err := ParseDay("2026-11-20"); err != nil || d.Format(DateLayout) != "2026-11-20" {
		t.Errorf("ParseDay = %v, %v", d, err)
	}
	for _, bad := range []string{"", "2026-11-20T09:00:00Z", "20 Nov 2026", "2026-13-01"} {
		if _, err := ParseDay(bad); err == nil {
			t.Errorf("ParseDay(%q) should fail", bad)
		}
	}
}
