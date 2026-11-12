package api

import (
	"encoding/json"
	"time"

	"github.com/spencercornish/kilnlab/internal/firing"
)

// Day is a firing's scheduled day on the wire: "2026-11-20". It's a calendar
// date, not an instant, so it never shifts with the viewer's timezone.
type Day time.Time

func (d Day) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Time(d).Format(firing.DateLayout))
}

func (d *Day) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	t, err := firing.ParseDay(s)
	if err != nil {
		return err
	}
	*d = Day(t)
	return nil
}
