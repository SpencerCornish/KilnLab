// Package firing is the pure domain logic for firings (spec §6, §7): the status
// state machine, the requests window, minimum notice, fullness, the
// compatibility check, and the profile-based phase timings. No I/O, so it is
// unit tested exhaustively.
package firing

import (
	"errors"
	"fmt"
	"slices"

	"github.com/spencercornish/kilnlab/internal/db"
	"github.com/spencercornish/kilnlab/internal/policy"
)

type Status = db.FiringStatus

const (
	Proposed  = db.FiringStatusProposed
	Scheduled = db.FiringStatusScheduled
	OnHold    = db.FiringStatusOnHold
	Firing    = db.FiringStatusFiring
	Cooling   = db.FiringStatusCooling
	Completed = db.FiringStatusCompleted
	Cancelled = db.FiringStatusCancelled
)

// AllStatuses in display order.
var AllStatuses = []Status{
	Proposed, Scheduled, OnHold, Firing, Cooling, Completed, Cancelled,
}

var (
	reschedulable = []Status{Proposed, OnHold, Scheduled}
	holdable      = []Status{Proposed, Scheduled}
	cancellable   = []Status{Proposed, OnHold, Scheduled}
	// Before the kiln starts, requests can still be approved/declined.
	decidable = []Status{Proposed, OnHold, Scheduled}
	// Anyone signed in may unload their work. Start is kiln-master only.
	memberTargets = []Status{Completed}
)

var (
	// ErrForbidden: the actor's roles don't allow this action.
	ErrForbidden = errors.New("not allowed")
	// ErrInvalidTransition: the action isn't possible from the current status.
	ErrInvalidTransition = errors.New("invalid status change")
)

// Actor is who is attempting an action on a particular firing.
type Actor struct {
	Roles policy.Roles
}

// State is the part of a firing the state machine needs.
type State struct {
	Status       Status
	WasScheduled bool
}

func invalid(from Status, what string) error {
	return fmt.Errorf("%w: can't %s a firing that is %s", ErrInvalidTransition, what, Label(from))
}

func requireManager(a Actor) error {
	if !a.Roles.CanManageFirings() {
		return fmt.Errorf("%w: only kiln masters can do that", ErrForbidden)
	}
	return nil
}

// Claim: a kiln master takes responsibility. Proposed becomes scheduled; an
// on-hold firing stays on hold; a scheduled/firing/cooling firing just gets an
// owner (legacy or takeover).
func Claim(s State, a Actor) (Status, error) {
	if err := requireManager(a); err != nil {
		return "", err
	}
	switch s.Status {
	case Proposed:
		return Scheduled, nil
	case Scheduled, OnHold, Firing, Cooling:
		return s.Status, nil
	default:
		return "", invalid(s.Status, "claim")
	}
}

// Unclaim: the owner drops responsibility before the kiln starts. A scheduled
// firing returns to proposed; an on-hold firing stays on hold.
func Unclaim(s State) (Status, error) {
	switch s.Status {
	case Proposed:
		return Proposed, nil
	case Scheduled:
		return Proposed, nil
	case OnHold:
		return OnHold, nil
	default:
		return "", invalid(s.Status, "unclaim")
	}
}

// Hold: proposed/scheduled → on_hold. The caller records WasScheduled so
// Release can return to the right place.
func Hold(s State, a Actor) (Status, error) {
	if err := requireManager(a); err != nil {
		return "", err
	}
	if !slices.Contains(holdable, s.Status) {
		return "", invalid(s.Status, "hold")
	}
	return OnHold, nil
}

// Release: on_hold → proposed, or → scheduled if it was scheduled before the
// hold.
func Release(s State, a Actor) (Status, error) {
	if err := requireManager(a); err != nil {
		return "", err
	}
	if s.Status != OnHold {
		return "", invalid(s.Status, "release")
	}
	if s.WasScheduled {
		return Scheduled, nil
	}
	return Proposed, nil
}

// Cancel: from any pre-firing status → cancelled.
func Cancel(s State, a Actor) (Status, error) {
	if err := requireManager(a); err != nil {
		return "", err
	}
	if !slices.Contains(cancellable, s.Status) {
		return "", invalid(s.Status, "cancel")
	}
	return Cancelled, nil
}

// Reschedule changes scheduled_date; the status is unchanged.
func Reschedule(s State, a Actor) error {
	if err := requireManager(a); err != nil {
		return err
	}
	if !slices.Contains(reschedulable, s.Status) {
		return invalid(s.Status, "reschedule")
	}
	return nil
}

// CanDecideRequests reports whether approve/decline is possible in this status.
func CanDecideRequests(s Status) bool { return slices.Contains(decidable, s) }

// IsActive reports whether management edits that don't change the status
// (fullness override, requests window) are still possible.
func IsActive(s Status) bool { return s != Completed && s != Cancelled }

// Advance moves a firing forward along the lifecycle.
//   - Scheduled → Firing: only kiln masters/admins.
//   - Firing/Cooling → Completed: anyone signed in (the physical unload).
//
// Cooling itself is written by the jobs runner, not by users. Unloading early
// (from Firing) is allowed.
func Advance(s State, to Status, a Actor) error {
	if to != Firing && to != Completed {
		return fmt.Errorf("%w: %s is not a status you can move to directly", ErrInvalidTransition, Label(to))
	}
	switch to {
	case Firing:
		if !a.Roles.CanStartFiring() {
			return fmt.Errorf("%w: only kiln masters can start the kiln", ErrForbidden)
		}
		if s.Status != Scheduled {
			return fmt.Errorf("%w: can't start a firing that is %s", ErrInvalidTransition, Label(s.Status))
		}
	case Completed:
		if s.Status != Firing && s.Status != Cooling {
			return fmt.Errorf("%w: can't unload a firing that is %s", ErrInvalidTransition, Label(s.Status))
		}
	}
	return nil
}

// Label is the human name of a status.
func Label(s Status) string {
	switch s {
	case OnHold:
		return "on hold"
	case Scheduled:
		return "scheduled"
	default:
		return string(s)
	}
}
