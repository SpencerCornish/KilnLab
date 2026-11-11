package firing

import (
	"errors"
	"slices"
	"testing"

	"github.com/spencercornish/kilnlab/internal/policy"
)

var (
	member     = Actor{}
	selfLoader = Actor{Roles: policy.Roles{SelfLoader: true}}
	loader     = Actor{Roles: policy.Roles{SelfLoader: true}, IsLoader: true}
	kilnMaster = Actor{Roles: policy.Roles{KilnMaster: true}}
	admin      = Actor{Roles: policy.Roles{Admin: true}}
	// Assigned as loader but no longer a self-loader: gets nothing.
	exLoader = Actor{IsLoader: true}
)

func TestManagementActions(t *testing.T) {
	type action func(State, Actor) (Status, error)
	actions := map[string]struct {
		fn      action
		allowed map[Status]Status // from → to
	}{
		"confirm": {Confirm, map[Status]Status{Proposed: Confirmed}},
		"hold":    {Hold, map[Status]Status{Proposed: OnHold, Confirmed: OnHold}},
		"release": {Release, map[Status]Status{OnHold: Proposed}},
		"cancel": {Cancel, map[Status]Status{
			Proposed: Cancelled, OnHold: Cancelled, Confirmed: Cancelled, Loading: Cancelled, Loaded: Cancelled,
		}},
	}
	for name, act := range actions {
		for _, from := range AllStatuses {
			for _, a := range []Actor{member, selfLoader, loader, kilnMaster, admin} {
				got, err := act.fn(State{Status: from}, a)
				want, ok := act.allowed[from]
				switch {
				case !a.Roles.CanManageFirings():
					if !errors.Is(err, ErrForbidden) {
						t.Errorf("%s from %s by %+v: err = %v, want forbidden", name, from, a, err)
					}
				case !ok:
					if !errors.Is(err, ErrInvalidTransition) {
						t.Errorf("%s from %s: err = %v, want invalid", name, from, err)
					}
				case err != nil || got != want:
					t.Errorf("%s from %s = %s, %v; want %s", name, from, got, err, want)
				}
			}
		}
	}
}

func TestReleaseReturnsToConfirmed(t *testing.T) {
	got, err := Release(State{Status: OnHold, WasConfirmed: true}, kilnMaster)
	if err != nil || got != Confirmed {
		t.Fatalf("release of held confirmed firing = %s, %v", got, err)
	}
}

func TestReschedule(t *testing.T) {
	for _, from := range AllStatuses {
		err := Reschedule(State{Status: from}, kilnMaster)
		want := slices.Contains([]Status{Proposed, OnHold, Confirmed}, from)
		if (err == nil) != want {
			t.Errorf("reschedule from %s: err = %v", from, err)
		}
		if err := Reschedule(State{Status: from}, member); !errors.Is(err, ErrForbidden) {
			t.Errorf("member reschedule from %s: err = %v", from, err)
		}
	}
}

func TestAdvanceKilnMasterCanSkipForward(t *testing.T) {
	for i, from := range lifecycle {
		for j, to := range lifecycle[1:] {
			err := Advance(State{Status: from}, to, kilnMaster)
			if forward := j+1 > i; (err == nil) != forward {
				t.Errorf("KM %s → %s: err = %v, want ok=%v", from, to, err, forward)
			}
		}
	}
}

func TestAdvanceRejectsNonLifecycle(t *testing.T) {
	for _, from := range []Status{Proposed, OnHold, Cancelled, Completed} {
		if err := Advance(State{Status: from}, Loading, kilnMaster); !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("KM %s → loading: err = %v, want invalid", from, err)
		}
	}
	for _, to := range []Status{Proposed, OnHold, Confirmed, Cancelled} {
		if err := Advance(State{Status: Confirmed}, to, kilnMaster); !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("advance to %s should be rejected, got %v", to, err)
		}
	}
}

func TestAdvanceSelfLoader(t *testing.T) {
	cases := []struct {
		from, to Status
		ok       bool
	}{
		{Confirmed, Loading, true},
		{Loading, Loaded, true},
		{Confirmed, Loaded, true}, // skipping loading is fine
		{Cooling, Unloaded, true},
		{Firing, Unloaded, true}, // skipping cooling is fine
		{Loaded, Firing, false},  // never start the kiln
		{Confirmed, Firing, false},
		{Loaded, Unloaded, false}, // can't skip past the start
		{Confirmed, Unloaded, false},
		{Firing, Cooling, false},
		{Unloaded, ReadyForPickup, false},
		{ReadyForPickup, Completed, false},
		{Loaded, Loading, false}, // backwards
	}
	for _, c := range cases {
		err := Advance(State{Status: c.from}, c.to, loader)
		if (err == nil) != c.ok {
			t.Errorf("loader %s → %s: err = %v, want ok=%v", c.from, c.to, err, c.ok)
		}
	}
}

func TestOnlyKilnMastersStartTheKiln(t *testing.T) {
	for _, a := range []Actor{member, selfLoader, loader, exLoader} {
		for _, from := range lifecycle {
			if err := Advance(State{Status: from}, Firing, a); !errors.Is(err, ErrForbidden) {
				t.Errorf("%+v start from %s: err = %v, want forbidden", a, from, err)
			}
		}
	}
	for _, a := range []Actor{kilnMaster, admin} {
		if err := Advance(State{Status: Loaded}, Firing, a); err != nil {
			t.Errorf("%+v start: %v", a, err)
		}
	}
}

func TestUnassignedCannotAdvance(t *testing.T) {
	for _, a := range []Actor{member, selfLoader, exLoader} {
		for _, to := range lifecycle[1:] {
			if err := Advance(State{Status: Confirmed}, to, a); !errors.Is(err, ErrForbidden) {
				t.Errorf("%+v confirmed → %s: err = %v, want forbidden", a, to, err)
			}
		}
	}
}

func TestDecideAndActive(t *testing.T) {
	for _, s := range AllStatuses {
		wantDecide := slices.Contains([]Status{Proposed, OnHold, Confirmed, Loading, Loaded}, s)
		if CanDecideRequests(s) != wantDecide {
			t.Errorf("CanDecideRequests(%s) = %v", s, !wantDecide)
		}
		if IsActive(s) != (s != Completed && s != Cancelled) {
			t.Errorf("IsActive(%s) wrong", s)
		}
	}
}
