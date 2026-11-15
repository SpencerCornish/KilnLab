package jobs

import (
	"testing"
	"time"

	"github.com/spencercornish/kilnlab/internal/firing"
)

var (
	start = time.Date(2026, 11, 12, 16, 0, 0, 0, time.UTC)
	rules = Rules{UnderfilledPct: 50, MissedStartGrace: 2 * time.Hour, MissedUnloadGrace: 3 * time.Hour}
)

func snap(status firing.Status, pct int) Snapshot {
	return Snapshot{
		Status: status, CreatedAt: start.Add(-72 * time.Hour), DayEnd: start,
		RequestsCloseAt: start.Add(-12 * time.Hour), FullnessPct: pct,
	}
}

func ptr(t time.Time) *time.Time { return &t }

func TestCloseRequests(t *testing.T) {
	closeAt := start.Add(-12 * time.Hour)
	cases := []struct {
		name string
		s    Snapshot
		now  time.Time
		want Result
	}{
		{"before cutoff", snap(firing.Confirmed, 80), closeAt.Add(-time.Minute), Result{}},
		{"at cutoff, full, confirmed", snap(firing.Confirmed, 80), closeAt, Result{CloseRequests: true}},
		{"under-filled", snap(firing.Confirmed, 30), closeAt, Result{CloseRequests: true, Underfilled: true}},
		{"threshold is exclusive", snap(firing.Confirmed, 50), closeAt, Result{CloseRequests: true}},
		{"unconfirmed", snap(firing.Proposed, 80), closeAt, Result{CloseRequests: true, NeedsConfirmation: true}},
		{"on hold + under-filled", snap(firing.OnHold, 10), closeAt,
			Result{CloseRequests: true, Underfilled: true, NeedsConfirmation: true}},
		{"already handled", func() Snapshot { s := snap(firing.Proposed, 10); s.CloseHandledAt = ptr(closeAt); return s }(),
			closeAt.Add(time.Hour), Result{}},
		{"closed manually early", func() Snapshot { s := snap(firing.Confirmed, 20); s.ClosedManually = true; return s }(),
			closeAt.Add(-24 * time.Hour), Result{CloseRequests: true, Underfilled: true}},
		{"cancelled", snap(firing.Cancelled, 10), closeAt, Result{}},
		{"created after its cutoff: no alerts", func() Snapshot {
			s := snap(firing.Proposed, 10)
			s.CreatedAt = closeAt.Add(time.Hour)
			return s
		}(), closeAt.Add(2 * time.Hour), Result{CloseRequests: true}},
	}
	for _, c := range cases {
		if got := Due(c.s, rules, c.now); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestMissedStart(t *testing.T) {
	handled := func(s Snapshot) Snapshot { s.CloseHandledAt = ptr(start); return s }
	for _, st := range []firing.Status{firing.Confirmed, firing.Loading, firing.Loaded} {
		s := handled(snap(st, 80))
		if Due(s, rules, start.Add(2*time.Hour)).MissedStart {
			t.Errorf("%s: due exactly at grace end", st)
		}
		if !Due(s, rules, start.Add(2*time.Hour+time.Second)).MissedStart {
			t.Errorf("%s: not due after grace", st)
		}
		s.MissedStartSentAt = ptr(start)
		if Due(s, rules, start.Add(5*time.Hour)).MissedStart {
			t.Errorf("%s: sent twice", st)
		}
	}
	for _, st := range []firing.Status{firing.Proposed, firing.OnHold, firing.Firing, firing.Cancelled} {
		if Due(handled(snap(st, 80)), rules, start.Add(5*time.Hour)).MissedStart {
			t.Errorf("%s: missed start shouldn't apply", st)
		}
	}
}

func TestDayReminder(t *testing.T) {
	at := time.Date(2026, 11, 12, 15, 0, 0, 0, time.UTC) // the reminder instant (e.g. 8am studio time)
	s := Snapshot{
		Status: firing.Confirmed, CreatedAt: at.Add(-72 * time.Hour),
		DayEnd: at.Add(16 * time.Hour), DayReminderAt: at,
	}
	if Due(s, rules, at.Add(-time.Minute)).DayReminder {
		t.Error("due before the reminder hour")
	}
	if !Due(s, rules, at).DayReminder {
		t.Error("not due at the reminder hour")
	}
	if !Due(s, rules, at.Add(time.Hour)).DayReminder {
		t.Error("not due later that morning")
	}
	s.DayReminderSentAt = ptr(at)
	if Due(s, rules, at.Add(time.Hour)).DayReminder {
		t.Error("sent twice")
	}
	// Scheduled at short notice after its own reminder instant: nobody needs reminding.
	late := s
	late.DayReminderSentAt = nil
	late.CreatedAt = at.Add(time.Hour)
	if Due(late, rules, at.Add(2*time.Hour)).DayReminder {
		t.Error("created after the reminder hour")
	}
	// Only pre-start statuses are worth reminding about.
	for _, st := range []firing.Status{firing.Firing, firing.Cooling, firing.Completed, firing.Cancelled} {
		c := s
		c.DayReminderSentAt = nil
		c.Status = st
		if Due(c, rules, at).DayReminder {
			t.Errorf("%s: shouldn't remind", st)
		}
	}
	for _, st := range []firing.Status{firing.Proposed, firing.OnHold, firing.Confirmed, firing.Loading, firing.Loaded} {
		c := s
		c.DayReminderSentAt = nil
		c.Status = st
		if !Due(c, rules, at).DayReminder {
			t.Errorf("%s: should remind", st)
		}
	}
	// No reminder instant set (zero): never due.
	z := s
	z.DayReminderSentAt = nil
	z.DayReminderAt = time.Time{}
	if Due(z, rules, at).DayReminder {
		t.Error("a zero reminder instant should not remind")
	}
}

func TestMissedUnload(t *testing.T) {
	unload := start.Add(30 * time.Hour)
	s := snap(firing.Firing, 80)
	if Due(s, rules, unload.Add(10*time.Hour)).MissedUnload {
		t.Error("no estimate: nothing to miss")
	}
	s.EstimatedUnloadAt = &unload
	if Due(s, rules, unload.Add(3*time.Hour)).MissedUnload {
		t.Error("due at grace end")
	}
	for _, st := range []firing.Status{firing.Firing, firing.Cooling} {
		s.Status = st
		if !Due(s, rules, unload.Add(3*time.Hour+time.Minute)).MissedUnload {
			t.Errorf("%s: not due after grace", st)
		}
	}
	s.Status = firing.Completed
	if Due(s, rules, unload.Add(10*time.Hour)).MissedUnload {
		t.Error("unloaded: shouldn't remind")
	}
	s.Status = firing.Cooling
	s.MissedUnloadSentAt = &unload
	if Due(s, rules, unload.Add(10*time.Hour)).MissedUnload {
		t.Error("sent twice")
	}
}
