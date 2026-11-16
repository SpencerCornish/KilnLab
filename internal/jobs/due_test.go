package jobs

import (
	"testing"
	"time"

	"github.com/spencercornish/kilnlab/internal/firing"
)

var (
	start = time.Date(2026, 11, 12, 16, 0, 0, 0, time.UTC)
	rules = Rules{
		UnderfilledPct: 50, MissedStartGrace: 2 * time.Hour,
		MissedUnloadGrace: 3 * time.Hour, EscalationGrace: 2 * time.Hour,
	}
)

func tp(t time.Time) *time.Time { return &t }

func snap(status firing.Status, pct int) Snapshot {
	return Snapshot{
		Status: status, HasOwner: true, CreatedAt: start.Add(-72 * time.Hour), DayEnd: start,
		RequestsCloseAt: start.Add(-12 * time.Hour), FullnessPct: pct,
	}
}

func TestCloseRequests(t *testing.T) {
	closeAt := start.Add(-12 * time.Hour)
	cases := []struct {
		name string
		s    Snapshot
		now  time.Time
		want Result
	}{
		{"before cutoff", snap(firing.Scheduled, 80), closeAt.Add(-time.Minute), Result{}},
		{"at cutoff, full, scheduled", snap(firing.Scheduled, 80), closeAt, Result{CloseRequests: true}},
		{"under-filled", snap(firing.Scheduled, 30), closeAt, Result{CloseRequests: true, Underfilled: true}},
		{"threshold is exclusive", snap(firing.Scheduled, 50), closeAt, Result{CloseRequests: true}},
		{"unclaimed", func() Snapshot { s := snap(firing.Scheduled, 80); s.HasOwner = false; return s }(),
			closeAt, Result{CloseRequests: true, NeedsOwner: true}},
		{"unclaimed proposed", func() Snapshot { s := snap(firing.Proposed, 80); s.HasOwner = false; return s }(),
			closeAt, Result{CloseRequests: true, NeedsOwner: true}},
		{"on hold + under-filled", func() Snapshot { s := snap(firing.OnHold, 10); s.HasOwner = false; return s }(),
			closeAt, Result{CloseRequests: true, Underfilled: true, NeedsOwner: true}},
		{"already handled", func() Snapshot { s := snap(firing.Proposed, 10); s.CloseHandledAt = tp(closeAt); return s }(),
			closeAt.Add(time.Hour), Result{}},
		{"closed manually early", func() Snapshot { s := snap(firing.Scheduled, 20); s.ClosedManually = true; return s }(),
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
	handled := func(s Snapshot) Snapshot { s.CloseHandledAt = tp(start); return s }
	s := handled(snap(firing.Scheduled, 80))
	if Due(s, rules, start.Add(2*time.Hour)).MissedStart {
		t.Error("due exactly at grace end")
	}
	if !Due(s, rules, start.Add(2*time.Hour+time.Second)).MissedStart {
		t.Error("not due after grace")
	}
	s.MissedStartSentAt = tp(start)
	if Due(s, rules, start.Add(5*time.Hour)).MissedStart {
		t.Error("sent twice")
	}
	for _, st := range []firing.Status{firing.Proposed, firing.OnHold, firing.Firing, firing.Cooling, firing.Cancelled} {
		if Due(handled(snap(st, 80)), rules, start.Add(5*time.Hour)).MissedStart {
			t.Errorf("%s: missed start shouldn't apply", st)
		}
	}
}

func TestMissedStartEscalation(t *testing.T) {
	s := snap(firing.Scheduled, 80)
	s.MissedStartSentAt = tp(start)
	if Due(s, rules, start.Add(2*time.Hour)).MissedStartEscalation {
		t.Error("escalation due exactly at grace end")
	}
	if !Due(s, rules, start.Add(2*time.Hour+time.Second)).MissedStartEscalation {
		t.Error("escalation not due after grace")
	}
	s.HasOwner = false
	if Due(s, rules, start.Add(3*time.Hour)).MissedStartEscalation {
		t.Error("unclaimed firing should not escalate; all KMs already know")
	}
	s.HasOwner = true
	s.MissedStartEscalatedAt = tp(start)
	if Due(s, rules, start.Add(3*time.Hour)).MissedStartEscalation {
		t.Error("escalated twice")
	}
}

func TestDayReminder(t *testing.T) {
	at := time.Date(2026, 11, 12, 15, 0, 0, 0, time.UTC)
	s := Snapshot{
		Status: firing.Scheduled, HasOwner: true, CreatedAt: at.Add(-72 * time.Hour),
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
	s.DayReminderSentAt = tp(at)
	if Due(s, rules, at.Add(time.Hour)).DayReminder {
		t.Error("sent twice")
	}
	late := s
	late.DayReminderSentAt = nil
	late.CreatedAt = at.Add(time.Hour)
	if Due(late, rules, at.Add(2*time.Hour)).DayReminder {
		t.Error("created after the reminder hour")
	}
	for _, st := range []firing.Status{firing.Firing, firing.Cooling, firing.Completed, firing.Cancelled} {
		c := s
		c.DayReminderSentAt = nil
		c.Status = st
		if Due(c, rules, at).DayReminder {
			t.Errorf("%s: shouldn't remind", st)
		}
	}
	for _, st := range []firing.Status{firing.Proposed, firing.OnHold, firing.Scheduled} {
		c := s
		c.DayReminderSentAt = nil
		c.Status = st
		if !Due(c, rules, at).DayReminder {
			t.Errorf("%s: should remind", st)
		}
	}
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
	s.UnloadableAt = &unload
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

func TestMissedUnloadEscalation(t *testing.T) {
	unload := start.Add(30 * time.Hour)
	s := snap(firing.Cooling, 80)
	s.UnloadableAt = &unload
	s.MissedUnloadSentAt = tp(start)
	if Due(s, rules, start.Add(2*time.Hour)).MissedUnloadEscalation {
		t.Error("escalation due exactly at grace end")
	}
	if !Due(s, rules, start.Add(2*time.Hour+time.Second)).MissedUnloadEscalation {
		t.Error("escalation not due after grace")
	}
	s.HasOwner = false
	if Due(s, rules, start.Add(3*time.Hour)).MissedUnloadEscalation {
		t.Error("unclaimed firing should not escalate")
	}
}

func TestAutoCooling(t *testing.T) {
	ends := start.Add(10 * time.Hour)
	s := snap(firing.Firing, 80)
	s.FiringEndsAt = &ends
	if Due(s, rules, ends.Add(-time.Second)).AutoCooling {
		t.Error("cooling before the estimate")
	}
	if !Due(s, rules, ends).AutoCooling {
		t.Error("cooling at the estimate")
	}
	if !Due(s, rules, ends.Add(time.Hour)).AutoCooling {
		t.Error("cooling after the estimate")
	}
	s.FiringEndsAt = nil
	if Due(s, rules, ends.Add(time.Hour)).AutoCooling {
		t.Error("no estimate: no auto cooling")
	}
	for _, st := range []firing.Status{firing.Scheduled, firing.Cooling, firing.Completed, firing.Cancelled} {
		s.Status = st
		s.FiringEndsAt = &ends
		if Due(s, rules, ends.Add(time.Hour)).AutoCooling {
			t.Errorf("%s: auto cooling shouldn't apply", st)
		}
	}
}
