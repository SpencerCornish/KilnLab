package firing

import (
	"errors"
	"slices"
	"testing"

	"github.com/spencercornish/kilnlab/internal/policy"
)

var (
	member     = Actor{} // anyone signed in; can unload work
	kilnMaster = Actor{Roles: policy.Roles{KilnMaster: true}}
	admin      = Actor{Roles: policy.Roles{Admin: true}}
)

func TestClaim(t *testing.T) {
	allowed := map[Status]Status{
		Proposed:  Scheduled,
		Scheduled: Scheduled,
		OnHold:    OnHold,
		Firing:    Firing,
		Cooling:   Cooling,
	}
	for _, from := range AllStatuses {
		for _, a := range []Actor{member, kilnMaster, admin} {
			got, err := Claim(State{Status: from}, a)
			want, ok := allowed[from]
			switch {
			case !a.Roles.CanManageFirings():
				if !errors.Is(err, ErrForbidden) {
					t.Errorf("claim from %s by %+v: err = %v, want forbidden", from, a, err)
				}
			case !ok:
				if !errors.Is(err, ErrInvalidTransition) {
					t.Errorf("claim from %s: err = %v, want invalid", from, err)
				}
			case err != nil || got != want:
				t.Errorf("claim from %s = %s, %v; want %s", from, got, err, want)
			}
		}
	}
}

func TestUnclaim(t *testing.T) {
	allowed := map[Status]Status{
		Proposed:  Proposed,
		Scheduled: Proposed,
		OnHold:    OnHold,
	}
	for _, from := range AllStatuses {
		got, err := Unclaim(State{Status: from})
		want, ok := allowed[from]
		if !ok {
			if !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("unclaim from %s: err = %v, want invalid", from, err)
			}
			continue
		}
		if err != nil || got != want {
			t.Errorf("unclaim from %s = %s, %v; want %s", from, got, err, want)
		}
	}
}

func TestManagementActions(t *testing.T) {
	type action func(State, Actor) (Status, error)
	actions := map[string]struct {
		fn      action
		allowed map[Status]Status // from → to
	}{
		"hold":    {Hold, map[Status]Status{Proposed: OnHold, Scheduled: OnHold}},
		"release": {Release, map[Status]Status{OnHold: Proposed}},
		"cancel":  {Cancel, map[Status]Status{Proposed: Cancelled, OnHold: Cancelled, Scheduled: Cancelled}},
	}
	for name, act := range actions {
		for _, from := range AllStatuses {
			for _, a := range []Actor{member, kilnMaster, admin} {
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

func TestReleaseReturnsToScheduled(t *testing.T) {
	got, err := Release(State{Status: OnHold, WasScheduled: true}, kilnMaster)
	if err != nil || got != Scheduled {
		t.Fatalf("release of held scheduled firing = %s, %v", got, err)
	}
}

func TestReschedule(t *testing.T) {
	for _, from := range AllStatuses {
		err := Reschedule(State{Status: from}, kilnMaster)
		want := slices.Contains([]Status{Proposed, OnHold, Scheduled}, from)
		if (err == nil) != want {
			t.Errorf("reschedule from %s: err = %v", from, err)
		}
		if err := Reschedule(State{Status: from}, member); !errors.Is(err, ErrForbidden) {
			t.Errorf("member reschedule from %s: err = %v", from, err)
		}
	}
}

func TestAdvance(t *testing.T) {
	cases := []struct {
		name string
		from Status
		to   Status
		a    Actor
		ok   bool
	}{
		{"KM starts", Scheduled, Firing, kilnMaster, true},
		{"admin starts", Scheduled, Firing, admin, true},
		{"member cannot start", Scheduled, Firing, member, false},
		{"start only from scheduled", Proposed, Firing, kilnMaster, false},
		{"cooling is automatic, not user-set", Scheduled, Cooling, kilnMaster, false},
		{"member unloads while firing", Firing, Completed, member, true},
		{"member unloads while cooling", Cooling, Completed, member, true},
		{"KM unloads while cooling", Cooling, Completed, kilnMaster, true},
		{"unload before start", Scheduled, Completed, member, false},
		{"unload from completed", Completed, Completed, member, false},
		{"backwards", Completed, Firing, kilnMaster, false},
	}
	for _, c := range cases {
		err := Advance(State{Status: c.from}, c.to, c.a)
		if (err == nil) != c.ok {
			t.Errorf("%s: %s → %s err = %v, want ok=%v", c.name, c.from, c.to, err, c.ok)
		}
	}
}

func TestDecideAndActive(t *testing.T) {
	for _, s := range AllStatuses {
		wantDecide := slices.Contains([]Status{Proposed, OnHold, Scheduled}, s)
		if CanDecideRequests(s) != wantDecide {
			t.Errorf("CanDecideRequests(%s) = %v", s, !wantDecide)
		}
		if IsActive(s) != (s != Completed && s != Cancelled) {
			t.Errorf("IsActive(%s) wrong", s)
		}
	}
}
