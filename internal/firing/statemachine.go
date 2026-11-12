// Package firing is the pure domain logic for firings (spec §6, §7): the status
// state machine, the requests window, minimum notice, fullness and the
// compatibility check. No I/O, so it is unit tested exhaustively.
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
	OnHold    = db.FiringStatusOnHold
	Confirmed = db.FiringStatusConfirmed
	Loading   = db.FiringStatusLoading
	Loaded    = db.FiringStatusLoaded
	Firing    = db.FiringStatusFiring
	Cooling   = db.FiringStatusCooling
	Completed = db.FiringStatusCompleted // unloaded, ready for pickup and done in one step
	Cancelled = db.FiringStatusCancelled
)

// AllStatuses in display order.
var AllStatuses = []Status{
	Proposed, OnHold, Confirmed, Loading, Loaded, Firing, Cooling, Completed, Cancelled,
}

// lifecycle is the forward path once a firing is confirmed. Kiln masters may
// skip ahead along it (e.g. confirmed → firing when nobody scanned loading).
var lifecycle = []Status{Confirmed, Loading, Loaded, Firing, Cooling, Completed}

// Statuses in which the firing can still be rescheduled, held, confirmed etc.
var (
	reschedulable = []Status{Proposed, OnHold, Confirmed}
	holdable      = []Status{Proposed, Confirmed}
	cancellable   = []Status{Proposed, OnHold, Confirmed, Loading, Loaded}
	// Before the kiln starts, requests can still be approved/declined.
	decidable = []Status{Proposed, OnHold, Confirmed, Loading, Loaded}
	// Anyone signed in may move a firing to these: they say they've loaded or
	// unloaded their work. Everything else is kiln masters only.
	memberTargets = []Status{Loading, Loaded, Completed}
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
	WasConfirmed bool
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

// Confirm: proposed → confirmed.
func Confirm(s State, a Actor) (Status, error) {
	if err := requireManager(a); err != nil {
		return "", err
	}
	if s.Status != Proposed {
		return "", invalid(s.Status, "confirm")
	}
	return Confirmed, nil
}

// Hold: proposed/confirmed → on_hold. The caller records WasConfirmed so
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

// Release: on_hold → proposed, or → confirmed if it was confirmed before the hold.
func Release(s State, a Actor) (Status, error) {
	if err := requireManager(a); err != nil {
		return "", err
	}
	if s.Status != OnHold {
		return "", invalid(s.Status, "release")
	}
	if s.WasConfirmed {
		return Confirmed, nil
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

// Advance moves a firing forward along the lifecycle (loading … completed).
// Completed is the unload step: the firing is unloaded and ready for pickup.
//   - Kiln masters may skip intermediate statuses.
//   - Anyone may move a firing to loading, loaded or completed, but never past
//     the kiln start (not even by skipping).
//   - Only kiln masters start the kiln.
func Advance(s State, to Status, a Actor) error {
	fromIdx := slices.Index(lifecycle, s.Status)
	toIdx := slices.Index(lifecycle, to)
	if toIdx <= 0 { // not a lifecycle target (confirmed is reached via Confirm/Release)
		return fmt.Errorf("%w: %s is not a status you can move to directly", ErrInvalidTransition, Label(to))
	}

	manager := a.Roles.CanManageFirings()
	member := slices.Contains(memberTargets, to)
	if to == Firing && !a.Roles.CanStartFiring() {
		return fmt.Errorf("%w: only kiln masters can start the kiln", ErrForbidden)
	}
	if !manager && !member {
		return fmt.Errorf("%w: you can't mark this firing %s", ErrForbidden, Label(to))
	}

	if fromIdx < 0 || toIdx <= fromIdx {
		return fmt.Errorf("%w: can't go from %s to %s", ErrInvalidTransition, Label(s.Status), Label(to))
	}
	if !manager {
		// Anyone can skip a step (e.g. confirmed → loaded) but never past the kiln start.
		firingIdx := slices.Index(lifecycle, Firing)
		if fromIdx < firingIdx && toIdx > firingIdx {
			return fmt.Errorf("%w: only kiln masters can start the kiln", ErrForbidden)
		}
	}
	return nil
}

// Label is the human name of a status.
func Label(s Status) string {
	switch s {
	case OnHold:
		return "on hold"
	default:
		return string(s)
	}
}
