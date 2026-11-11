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
		{"KM confirmed: load or start", Confirmed, kilnMaster, []Status{Loading, Firing}},
		{"KM loading: loaded or start", Loading, kilnMaster, []Status{Loaded, Firing}},
		{"KM loaded: start", Loaded, kilnMaster, []Status{Firing}},
		{"KM firing", Firing, kilnMaster, []Status{Cooling}},
		{"KM cooling", Cooling, kilnMaster, []Status{Unloaded}},
		{"KM unloaded", Unloaded, kilnMaster, []Status{ReadyForPickup}},
		{"KM ready", ReadyForPickup, kilnMaster, []Status{Completed}},
		{"KM completed", Completed, kilnMaster, nil},
		{"KM proposed (confirm first)", Proposed, kilnMaster, nil},
		{"admin loaded", Loaded, admin, []Status{Firing}},
		{"loader confirmed", Confirmed, loader, []Status{Loading}},
		{"loader loading", Loading, loader, []Status{Loaded}},
		{"loader loaded", Loaded, loader, nil},
		{"loader firing: skip cooling", Firing, loader, []Status{Unloaded}},
		{"loader cooling", Cooling, loader, []Status{Unloaded}},
		{"loader unloaded", Unloaded, loader, nil},
		{"member", Confirmed, member, nil},
		{"unassigned self-loader", Confirmed, selfLoader, nil},
	}
	for _, c := range cases {
		got := NextSteps(State{Status: c.from}, c.a)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
		// Every offered step must actually be allowed.
		for _, to := range got {
			if err := Advance(State{Status: c.from}, to, c.a); err != nil {
				t.Errorf("%s: offered %s but Advance says %v", c.name, to, err)
			}
		}
	}
}

func TestBlockedReason(t *testing.T) {
	if r := BlockedReason(State{Status: Loaded}, loader); r != "Only kiln masters can start the kiln." {
		t.Errorf("loader at loaded: %q", r)
	}
	if r := BlockedReason(State{Status: Unloaded}, loader); r != "A kiln master needs to do the next step." {
		t.Errorf("loader at unloaded: %q", r)
	}
	if r := BlockedReason(State{Status: Confirmed}, member); r == "" {
		t.Error("member should get a reason")
	}
	if r := BlockedReason(State{Status: Confirmed}, kilnMaster); r != "" {
		t.Errorf("KM has actions, got %q", r)
	}
	for _, s := range []Status{Completed, Cancelled, Proposed} {
		if r := BlockedReason(State{Status: s}, member); r != "" {
			t.Errorf("%s: %q", s, r)
		}
	}
}

func TestRelevantAtKiln(t *testing.T) {
	now := t0
	for _, s := range AllStatuses {
		got := RelevantAtKiln(s, now.Add(72*time.Hour), now)
		want := slices.Contains([]Status{Loading, Loaded, Firing, Cooling, Unloaded}, s)
		if got != want {
			t.Errorf("%s far from start: %v", s, got)
		}
	}
	for _, d := range []time.Duration{-24 * time.Hour, -time.Hour, 0, 23 * time.Hour, 24 * time.Hour} {
		if !RelevantAtKiln(Confirmed, now.Add(d), now) {
			t.Errorf("confirmed at %v should be relevant", d)
		}
	}
	for _, d := range []time.Duration{-25 * time.Hour, 25 * time.Hour} {
		if RelevantAtKiln(Confirmed, now.Add(d), now) {
			t.Errorf("confirmed at %v should not be relevant", d)
		}
	}
	if RelevantAtKiln(Proposed, now, now) {
		t.Error("proposed firings aren't relevant at the kiln")
	}
}

func TestTimestampField(t *testing.T) {
	for _, s := range AllStatuses {
		f := TimestampField(s)
		if (f != "") != slices.Contains([]Status{Firing, Cooling, Unloaded, ReadyForPickup, Completed}, s) {
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
	// Gaps (skipped statuses) are fine.
	if err := (Timestamps{StartedAt: at(0), ReadyForPickupAt: at(30)}).CheckOrder(); err != nil {
		t.Error(err)
	}
	if err := (Timestamps{StartedAt: at(5), ReadyForPickupAt: at(3)}).CheckOrder(); err == nil {
		t.Error("ready before started should fail")
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

func TestEstimatedUnload(t *testing.T) {
	if got := EstimatedUnload(t0, 600); !got.Equal(t0.Add(10 * time.Hour)) {
		t.Errorf("got %v", got)
	}
}
