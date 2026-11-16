package firing

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func TestNextSteps(t *testing.T) {
	cases := []struct {
		name string
		from Status
		a    Actor
		want []Status
	}{
		{"KM scheduled: start", Scheduled, kilnMaster, []Status{Firing}},
		{"admin scheduled: start", Scheduled, admin, []Status{Firing}},
		{"member scheduled: blocked", Scheduled, member, nil},
		{"KM firing: unload", Firing, kilnMaster, []Status{Completed}},
		{"member firing: unload", Firing, member, []Status{Completed}},
		{"KM cooling: unload", Cooling, kilnMaster, []Status{Completed}},
		{"member cooling: unload", Cooling, member, []Status{Completed}},
		{"KM completed", Completed, kilnMaster, nil},
		{"KM proposed (take first)", Proposed, kilnMaster, nil},
		{"KM on hold (release first)", OnHold, kilnMaster, nil},
	}
	for _, c := range cases {
		got := NextSteps(State{Status: c.from}, c.a)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
		for _, to := range got {
			if err := Advance(State{Status: c.from}, to, c.a); err != nil {
				t.Errorf("%s: offered %s but Advance says %v", c.name, to, err)
			}
		}
	}
}

func TestBlockedReason(t *testing.T) {
	if r := BlockedReason(State{Status: Scheduled}, member); r != "Only kiln masters can start the kiln." {
		t.Errorf("member at scheduled: %q", r)
	}
	if r := BlockedReason(State{Status: Scheduled}, kilnMaster); r != "" {
		t.Errorf("KM has actions, got %q", r)
	}
	for _, s := range []Status{Completed, Cancelled, Proposed, OnHold} {
		if r := BlockedReason(State{Status: s}, member); r != "" {
			t.Errorf("%s: %q", s, r)
		}
	}
}

func TestRelevantAtKiln(t *testing.T) {
	now := t0
	for _, s := range AllStatuses {
		got := RelevantAtKiln(s, now.Add(72*time.Hour), now)
		want := slices.Contains([]Status{Firing, Cooling}, s)
		if got != want {
			t.Errorf("%s far from start: %v", s, got)
		}
	}
	for _, s := range []Status{Proposed, OnHold, Scheduled} {
		for _, d := range []time.Duration{-24 * time.Hour, -time.Hour, 0, 23 * time.Hour, 24 * time.Hour} {
			if !RelevantAtKiln(s, now.Add(d), now) {
				t.Errorf("%s at %v should be relevant", s, d)
			}
		}
		for _, d := range []time.Duration{-25 * time.Hour, 25 * time.Hour} {
			if RelevantAtKiln(s, now.Add(d), now) {
				t.Errorf("%s at %v should not be relevant", s, d)
			}
		}
	}
	for _, s := range []Status{Completed, Cancelled} {
		if RelevantAtKiln(s, now, now) {
			t.Errorf("%s should not be relevant at the kiln", s)
		}
	}
}

func TestTimestampField(t *testing.T) {
	for _, s := range AllStatuses {
		f := TimestampField(s)
		if (f != "") != slices.Contains([]Status{Firing, Cooling, Completed}, s) {
			t.Errorf("%s → %q", s, f)
		}
		if f != "" && !slices.Contains(CorrectableFields, f) {
			t.Errorf("%s field %q not correctable", s, f)
		}
	}
}

func TestTimestampsOrder(t *testing.T) {
	at := func(h int) *time.Time { v := t0.Add(time.Duration(h) * time.Hour); return &v }
	ok := Timestamps{StartedAt: at(0), CoolingAt: at(8), UnloadedAt: at(20)}
	if err := ok.CheckOrder(); err != nil {
		t.Fatal(err)
	}
	if got := ok.Latest(); !got.Equal(*at(20)) {
		t.Errorf("latest = %v", got)
	}
	if err := ok.With("cooling_at", *at(-1)).CheckOrder(); err == nil || err.Error() != "cooling can't be before started" {
		t.Errorf("cooling before start: %v", err)
	}
	// Gaps (skipped cooling) are fine.
	if err := (Timestamps{StartedAt: at(0), UnloadedAt: at(30)}).CheckOrder(); err != nil {
		t.Error(err)
	}
	if err := (Timestamps{StartedAt: at(5), UnloadedAt: at(3)}).CheckOrder(); err == nil {
		t.Error("unloaded before started should fail")
	}
	if (Timestamps{}).Latest() != nil {
		t.Error("empty latest should be nil")
	}
	if ok.Get("unloaded_at") != ok.UnloadedAt || ok.Get("nope") != nil {
		t.Error("Get")
	}
}

func TestCheckOccurredAt(t *testing.T) {
	if err := CheckOccurredAt(t0, t0); err != nil {
		t.Error(err)
	}
	if err := CheckOccurredAt(t0.Add(4*time.Minute), t0); err != nil {
		t.Errorf("small clock skew: %v", err)
	}
	if err := CheckOccurredAt(t0.Add(10*time.Minute), t0); !errors.Is(err, ErrBadTime) {
		t.Errorf("future: %v", err)
	}
	if err := CheckOccurredAt(t0.Add(-6*24*time.Hour), t0); err != nil {
		t.Errorf("6 days back: %v", err)
	}
	if err := CheckOccurredAt(t0.Add(-8*24*time.Hour), t0); !errors.Is(err, ErrBadTime) {
		t.Errorf("8 days back: %v", err)
	}
}

func TestPlannedTimingsAndReady(t *testing.T) {
	firingEnds, unloadable := PlannedTimings(t0, 600, 300)
	if !firingEnds.Equal(t0.Add(10 * time.Hour)) {
		t.Errorf("firing ends = %v", firingEnds)
	}
	if !unloadable.Equal(t0.Add(15 * time.Hour)) {
		t.Errorf("unloadable = %v", unloadable)
	}
	if ReadyToUnload(Scheduled, &unloadable, unloadable) {
		t.Error("scheduled firing should not be ready to unload")
	}
	if !ReadyToUnload(Cooling, &unloadable, unloadable) {
		t.Error("cooling firing should be ready at the estimate")
	}
	if ReadyToUnload(Cooling, &unloadable, unloadable.Add(-time.Minute)) {
		t.Error("cooling firing should not be ready before the estimate")
	}
}
