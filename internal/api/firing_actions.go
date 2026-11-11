package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/spencercornish/kilnlab/internal/db"
	"github.com/spencercornish/kilnlab/internal/firing"
)

// mutationResponse is the fresh firing detail plus any non-blocking warnings.
type mutationResponse struct {
	*firingDetailDTO
	Warnings []string `json:"warnings"`
}

type mutateFn func(ctx context.Context, q *db.Queries, f db.Firing, u *currentUser) (warnings []string, err error)

// mutateFiring locks the firing in {id} for the length of a transaction, runs
// fn, and responds with the updated detail. M3 will enqueue notifications in
// the same transaction.
func (s *Server) mutateFiring(w http.ResponseWriter, r *http.Request, fn mutateFn) {
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	u := userFromContext(r.Context())
	var warnings []string
	err = db.InTx(r.Context(), s.pool, func(q *db.Queries) error {
		f, err := q.GetFiringForUpdate(r.Context(), id)
		if err != nil {
			return err
		}
		warnings, err = fn(r.Context(), q, f, u)
		return err
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d, err := s.firingDetail(r.Context(), s.queries, id, u)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if warnings == nil {
		warnings = []string{}
	}
	writeJSON(w, http.StatusOK, mutationResponse{d, warnings})
}

// ── Requests ────────────────────────────────────────────────────────────────

type requestInput struct {
	Description string  `json:"description"`
	SharePct    int32   `json:"share_pct"`
	Cone        string  `json:"cone"`
	ClayBody    *string `json:"clay_body"`
	Glazed      bool    `json:"glazed"`
}

type validRequest struct {
	description string
	clayBody    *string
	compat      *string
}

func (s *Server) validateRequest(ctx context.Context, q *db.Queries, f db.Firing, in requestInput) (validRequest, error) {
	var v validRequest
	st, err := q.GetSettings(ctx)
	if err != nil {
		return v, err
	}
	if err := checkShare(in.SharePct); err != nil {
		return v, err
	}
	if err := checkCone(st, in.Cone); err != nil {
		return v, err
	}
	if v.description, err = text("description", in.Description, maxDescription, true); err != nil {
		return v, err
	}
	if v.clayBody, err = optText("clay body", in.ClayBody, maxClayBody); err != nil {
		return v, err
	}
	if c := firing.Compatibility(f.FiringType, f.Cone, in.Cone, in.Glazed); c != "" {
		v.compat = &c
	}
	return v, nil
}

// requestWarnings: compatibility and "may not fit", shown to the requester.
func requestWarnings(f db.Firing, approved []int32, share int32, compat *string) []string {
	var out []string
	if compat != nil {
		out = append(out, *compat+" The kiln master will decide.")
	}
	full := firing.ComputeFullness(f.ProposerSharePct, approved, nil)
	if firing.WouldOverfill(full.Calculated, share) {
		out = append(out, "This may not fit. The kiln master will decide.")
	}
	return out
}

func requireRequestsOpen(s *Server, f db.Firing) error {
	if !firing.RequestsOpen(window(f), s.now()) {
		return conflict("requests are closed for this firing")
	}
	return nil
}

func (s *Server) handleCreateRequest(w http.ResponseWriter, r *http.Request) {
	var in requestInput
	if !s.readJSON(w, r, &in) {
		return
	}
	s.mutateFiring(w, r, func(ctx context.Context, q *db.Queries, f db.Firing, u *currentUser) ([]string, error) {
		if f.ProposedByUserID == u.ID {
			return nil, conflict("you proposed this firing, so your work is already in it")
		}
		if err := requireRequestsOpen(s, f); err != nil {
			return nil, err
		}
		if has, err := q.UserHasLiveRequest(ctx, db.UserHasLiveRequestParams{FiringID: f.ID, UserID: u.ID}); err != nil {
			return nil, err
		} else if has {
			return nil, conflict("you already have a request in this firing")
		}
		v, err := s.validateRequest(ctx, q, f, in)
		if err != nil {
			return nil, err
		}
		approved, err := q.ApprovedShares(ctx, f.ID)
		if err != nil {
			return nil, err
		}
		if _, err := q.CreateRequest(ctx, db.CreateRequestParams{
			FiringID: f.ID, UserID: u.ID, Description: v.description, EstimatedSharePct: in.SharePct,
			Cone: in.Cone, ClayBody: v.clayBody, Glazed: in.Glazed, CompatibilityWarning: v.compat,
		}); err != nil {
			return nil, err
		}
		note := fmt.Sprintf("%s requested %d%%", u.DisplayName, in.SharePct)
		return requestWarnings(f, approved, in.SharePct, v.compat),
			s.logEvent(ctx, q, f, u, firing.EventRequestSubmitted, nil, nil, &note)
	})
}

// lockOwnRequest loads request {rid} in firing f and checks u may change it.
func (s *Server) lockOwnRequest(ctx context.Context, r *http.Request, q *db.Queries, f db.Firing, u *currentUser) (db.FiringRequest, error) {
	rid, err := idParam(r, "rid")
	if err != nil {
		return db.FiringRequest{}, err
	}
	rq, err := q.GetRequestForUpdate(ctx, rid)
	if err != nil {
		return rq, err
	}
	if rq.FiringID != f.ID {
		return rq, errNotFound
	}
	if rq.UserID != u.ID {
		return rq, forbidden("that isn't your request")
	}
	if rq.Status != db.RequestStatusPending && rq.Status != db.RequestStatusApproved {
		return rq, conflict("this request is %s", rq.Status)
	}
	return rq, requireRequestsOpen(s, f)
}

func (s *Server) handleUpdateRequest(w http.ResponseWriter, r *http.Request) {
	var in requestInput
	if !s.readJSON(w, r, &in) {
		return
	}
	s.mutateFiring(w, r, func(ctx context.Context, q *db.Queries, f db.Firing, u *currentUser) ([]string, error) {
		rq, err := s.lockOwnRequest(ctx, r, q, f, u)
		if err != nil {
			return nil, err
		}
		v, err := s.validateRequest(ctx, q, f, in)
		if err != nil {
			return nil, err
		}
		approved, err := q.ApprovedShares(ctx, f.ID)
		if err != nil {
			return nil, err
		}
		if rq.Status == db.RequestStatusApproved {
			// It's going back to pending, so it no longer counts towards fullness.
			approved = removeOne(approved, rq.EstimatedSharePct)
		}
		if _, err := q.UpdateRequest(ctx, db.UpdateRequestParams{
			ID: rq.ID, Description: v.description, EstimatedSharePct: in.SharePct, Cone: in.Cone,
			ClayBody: v.clayBody, Glazed: in.Glazed, CompatibilityWarning: v.compat,
		}); err != nil {
			return nil, err
		}
		warnings := requestWarnings(f, approved, in.SharePct, v.compat)
		if rq.Status == db.RequestStatusApproved {
			warnings = append(warnings, "Your request was approved before; it now needs approving again.")
		}
		note := fmt.Sprintf("%s updated their request (%d%%)", u.DisplayName, in.SharePct)
		return warnings, s.logEvent(ctx, q, f, u, firing.EventRequestUpdated, nil, nil, &note)
	})
}

func removeOne(xs []int32, v int32) []int32 {
	for i, x := range xs {
		if x == v {
			return append(xs[:i:i], xs[i+1:]...)
		}
	}
	return xs
}

func (s *Server) handleWithdrawRequest(w http.ResponseWriter, r *http.Request) {
	s.mutateFiring(w, r, func(ctx context.Context, q *db.Queries, f db.Firing, u *currentUser) ([]string, error) {
		rq, err := s.lockOwnRequest(ctx, r, q, f, u)
		if err != nil {
			return nil, err
		}
		if _, err := q.WithdrawRequest(ctx, rq.ID); err != nil {
			return nil, err
		}
		note := fmt.Sprintf("%s withdrew their request", u.DisplayName)
		return nil, s.logEvent(ctx, q, f, u, firing.EventRequestWithdrawn, nil, nil, &note)
	})
}

type decideInput struct {
	Decision string  `json:"decision"` // approve | decline
	Note     *string `json:"note"`
}

func (s *Server) handleDecideRequest(w http.ResponseWriter, r *http.Request) {
	var in decideInput
	if !s.readJSON(w, r, &in) {
		return
	}
	s.mutateFiring(w, r, func(ctx context.Context, q *db.Queries, f db.Firing, u *currentUser) ([]string, error) {
		if !u.Roles.CanDecideRequests(f.ProposedByUserID == u.ID) {
			return nil, forbidden("only kiln masters can approve or decline requests")
		}
		if !firing.CanDecideRequests(f.Status) {
			return nil, conflict("requests can't be changed once a firing is %s", firing.Label(f.Status))
		}
		var status db.RequestStatus
		var event string
		switch in.Decision {
		case "approve":
			status, event = db.RequestStatusApproved, firing.EventRequestApproved
		case "decline":
			status, event = db.RequestStatusDeclined, firing.EventRequestDeclined
		default:
			return nil, badRequest("decision must be approve or decline")
		}
		note, err := optText("note", in.Note, maxReason)
		if err != nil {
			return nil, err
		}
		rid, err := idParam(r, "rid")
		if err != nil {
			return nil, err
		}
		rq, err := q.GetRequestForUpdate(ctx, rid)
		if err != nil {
			return nil, err
		}
		if rq.FiringID != f.ID {
			return nil, errNotFound
		}
		if rq.Status == db.RequestStatusWithdrawn {
			return nil, conflict("this request was withdrawn")
		}

		var warnings []string
		if status == db.RequestStatusApproved && rq.Status != db.RequestStatusApproved {
			approved, err := q.ApprovedShares(ctx, f.ID)
			if err != nil {
				return nil, err
			}
			full := firing.ComputeFullness(f.ProposerSharePct, approved, nil)
			if firing.WouldOverfill(full.Calculated, rq.EstimatedSharePct) {
				warnings = append(warnings, fmt.Sprintf("Approved requests now add up to %d%%.", full.Calculated+int(rq.EstimatedSharePct)))
			}
		}
		if _, err := q.DecideRequest(ctx, db.DecideRequestParams{
			ID: rq.ID, Status: status, DecisionNote: note, DecidedByUserID: &u.ID,
		}); err != nil {
			return nil, err
		}
		// The timeline is visible to every member, so the decision note stays out of it.
		name, err := q.GetUserName(ctx, rq.UserID)
		if err != nil {
			return nil, err
		}
		verb := map[db.RequestStatus]string{db.RequestStatusApproved: "Approved", db.RequestStatusDeclined: "Declined"}[status]
		ev := fmt.Sprintf("%s %s's request (%d%%)", verb, name, rq.EstimatedSharePct)
		return warnings, s.logEvent(ctx, q, f, u, event, nil, nil, &ev)
	})
}

// ── Kiln master actions ─────────────────────────────────────────────────────

func actorFor(f db.Firing, u *currentUser) firing.Actor {
	return firing.Actor{Roles: u.Roles, IsLoader: f.LoaderUserID != nil && *f.LoaderUserID == u.ID}
}

func stateOf(f db.Firing) firing.State {
	return firing.State{Status: f.Status, WasConfirmed: f.WasConfirmed}
}

// changeStatus applies a state machine result and writes the event.
func (s *Server) changeStatus(ctx context.Context, q *db.Queries, f db.Firing, u *currentUser, to db.FiringStatus, holdReason, cancelReason, note *string) (db.Firing, error) {
	wasConfirmed := f.WasConfirmed
	if to == firing.OnHold {
		wasConfirmed = f.Status == firing.Confirmed
	}
	if to != firing.OnHold {
		holdReason = nil
	}
	updated, err := q.SetFiringStatus(ctx, db.SetFiringStatusParams{
		ID: f.ID, Status: to, WasConfirmed: wasConfirmed, HoldReason: holdReason, CancelReason: cancelReason,
	})
	if err != nil {
		return updated, err
	}
	return updated, s.logEvent(ctx, q, f, u, firing.EventStatusChange, &f.Status, &to, note)
}

func (s *Server) handleConfirm(w http.ResponseWriter, r *http.Request) {
	s.mutateFiring(w, r, func(ctx context.Context, q *db.Queries, f db.Firing, u *currentUser) ([]string, error) {
		to, err := firing.Confirm(stateOf(f), actorFor(f, u))
		if err != nil {
			return nil, err
		}
		_, err = s.changeStatus(ctx, q, f, u, to, nil, nil, nil)
		return nil, err
	})
}

type reasonInput struct {
	Reason string `json:"reason"`
}

func (s *Server) handleHold(w http.ResponseWriter, r *http.Request) {
	var in reasonInput
	if !s.readJSON(w, r, &in) {
		return
	}
	s.mutateFiring(w, r, func(ctx context.Context, q *db.Queries, f db.Firing, u *currentUser) ([]string, error) {
		to, err := firing.Hold(stateOf(f), actorFor(f, u))
		if err != nil {
			return nil, err
		}
		reason, err := text("reason", in.Reason, maxReason, true)
		if err != nil {
			return nil, err
		}
		_, err = s.changeStatus(ctx, q, f, u, to, &reason, nil, &reason)
		return nil, err
	})
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	var in reasonInput
	if !s.readJSON(w, r, &in) {
		return
	}
	s.mutateFiring(w, r, func(ctx context.Context, q *db.Queries, f db.Firing, u *currentUser) ([]string, error) {
		to, err := firing.Cancel(stateOf(f), actorFor(f, u))
		if err != nil {
			return nil, err
		}
		reason, err := text("reason", in.Reason, maxReason, true)
		if err != nil {
			return nil, err
		}
		_, err = s.changeStatus(ctx, q, f, u, to, nil, &reason, &reason)
		return nil, err
	})
}

type rescheduleInput struct {
	ScheduledStart *time.Time `json:"scheduled_start"`
	Note           *string    `json:"note"`
}

// reschedule moves the start (and the automatic requests cutoff with it).
func (s *Server) reschedule(ctx context.Context, q *db.Queries, f db.Firing, u *currentUser, start time.Time, note *string) error {
	start = start.UTC()
	if !start.After(s.now()) {
		return badRequest("the new start time must be in the future")
	}
	st, err := q.GetSettings(ctx)
	if err != nil {
		return err
	}
	if _, err := q.RescheduleFiring(ctx, db.RescheduleFiringParams{
		ID: f.ID, ScheduledStart: start, RequestsCloseAt: firing.RequestsCloseAt(start, st.RequestCutoffHours),
	}); err != nil {
		return err
	}
	loc, err := time.LoadLocation(st.Timezone)
	if err != nil {
		loc = time.UTC
	}
	const layout = "Mon Jan 2, 3:04 PM"
	ev := fmt.Sprintf("Moved from %s to %s", f.ScheduledStart.In(loc).Format(layout), start.In(loc).Format(layout))
	if note != nil {
		ev += ": " + *note
	}
	return s.logEvent(ctx, q, f, u, firing.EventRescheduled, nil, nil, &ev)
}

// handleRelease: on_hold → proposed/confirmed, optionally with a new start.
func (s *Server) handleRelease(w http.ResponseWriter, r *http.Request) {
	var in rescheduleInput
	if !s.readJSON(w, r, &in) {
		return
	}
	s.mutateFiring(w, r, func(ctx context.Context, q *db.Queries, f db.Firing, u *currentUser) ([]string, error) {
		to, err := firing.Release(stateOf(f), actorFor(f, u))
		if err != nil {
			return nil, err
		}
		note, err := optText("note", in.Note, maxReason)
		if err != nil {
			return nil, err
		}
		if in.ScheduledStart != nil && !in.ScheduledStart.Equal(f.ScheduledStart) {
			if err := s.reschedule(ctx, q, f, u, *in.ScheduledStart, nil); err != nil {
				return nil, err
			}
		}
		_, err = s.changeStatus(ctx, q, f, u, to, nil, nil, note)
		return nil, err
	})
}

func (s *Server) handleReschedule(w http.ResponseWriter, r *http.Request) {
	var in rescheduleInput
	if !s.readJSON(w, r, &in) {
		return
	}
	s.mutateFiring(w, r, func(ctx context.Context, q *db.Queries, f db.Firing, u *currentUser) ([]string, error) {
		if err := firing.Reschedule(stateOf(f), actorFor(f, u)); err != nil {
			return nil, err
		}
		if in.ScheduledStart == nil {
			return nil, badRequest("new start time is required")
		}
		note, err := optText("note", in.Note, maxReason)
		if err != nil {
			return nil, err
		}
		var warnings []string
		if f.RequestsClosedManually {
			warnings = append(warnings, "Requests are still closed; reopen them if you want more work.")
		}
		return warnings, s.reschedule(ctx, q, f, u, *in.ScheduledStart, note)
	})
}

// requireManage: kiln master, and the firing hasn't finished.
func requireManage(f db.Firing, u *currentUser) error {
	if !u.Roles.CanManageFirings() {
		return forbidden("only kiln masters can do that")
	}
	if !firing.IsActive(f.Status) {
		return conflict("this firing is %s", firing.Label(f.Status))
	}
	return nil
}

type fullnessInput struct {
	OverridePct *int32 `json:"override_pct"` // null clears the override
}

func (s *Server) handleFullnessOverride(w http.ResponseWriter, r *http.Request) {
	var in fullnessInput
	if !s.readJSON(w, r, &in) {
		return
	}
	s.mutateFiring(w, r, func(ctx context.Context, q *db.Queries, f db.Firing, u *currentUser) ([]string, error) {
		if err := requireManage(f, u); err != nil {
			return nil, err
		}
		if in.OverridePct != nil && (*in.OverridePct < 0 || *in.OverridePct > 100) {
			return nil, badRequest("fullness must be between 0 and 100%%")
		}
		if _, err := q.SetFullnessOverride(ctx, db.SetFullnessOverrideParams{ID: f.ID, OverridePct: in.OverridePct}); err != nil {
			return nil, err
		}
		note := "Cleared fullness override"
		if in.OverridePct != nil {
			note = fmt.Sprintf("Set fullness to %d%%", *in.OverridePct)
		}
		return nil, s.logEvent(ctx, q, f, u, firing.EventFullnessOverride, nil, nil, &note)
	})
}

type requestsWindowInput struct {
	Closed bool `json:"closed"`
}

func (s *Server) handleRequestsWindow(w http.ResponseWriter, r *http.Request) {
	var in requestsWindowInput
	if !s.readJSON(w, r, &in) {
		return
	}
	s.mutateFiring(w, r, func(ctx context.Context, q *db.Queries, f db.Firing, u *currentUser) ([]string, error) {
		if err := requireManage(f, u); err != nil {
			return nil, err
		}
		closeAt, event := f.RequestsCloseAt, firing.EventRequestsClosed
		if !in.Closed {
			closeAt, event = firing.ReopenCloseAt(f.RequestsCloseAt, f.ScheduledStart, s.now()), firing.EventRequestsReopened
		}
		updated, err := q.SetRequestsWindow(ctx, db.SetRequestsWindowParams{
			ID: f.ID, ClosedManually: in.Closed, RequestsCloseAt: closeAt,
		})
		if err != nil {
			return nil, err
		}
		var warnings []string
		if !in.Closed && !firing.RequestsOpen(window(updated), s.now()) {
			warnings = append(warnings, fmt.Sprintf("Requests still can't be made while the firing is %s.", firing.Label(f.Status)))
		}
		return warnings, s.logEvent(ctx, q, f, u, event, nil, nil, nil)
	})
}

type loaderInput struct {
	UserID *int64 `json:"user_id"` // null unassigns
}

func (s *Server) handleAssignLoader(w http.ResponseWriter, r *http.Request) {
	var in loaderInput
	if !s.readJSON(w, r, &in) {
		return
	}
	s.mutateFiring(w, r, func(ctx context.Context, q *db.Queries, f db.Firing, u *currentUser) ([]string, error) {
		if err := requireManage(f, u); err != nil {
			return nil, err
		}
		note := "Unassigned the loader"
		if in.UserID != nil {
			lu, err := q.GetUser(ctx, *in.UserID)
			if err != nil {
				return nil, badRequest("unknown user")
			}
			roles, err := s.rolesFor(ctx, q, lu)
			if err != nil {
				return nil, err
			}
			if !roles.CanBeLoader() {
				return nil, badRequest("%s isn't a self-loader or kiln master", lu.DisplayName)
			}
			note = "Assigned " + lu.DisplayName + " as loader"
		}
		if _, err := q.SetLoader(ctx, db.SetLoaderParams{ID: f.ID, LoaderUserID: in.UserID}); err != nil {
			return nil, err
		}
		return nil, s.logEvent(ctx, q, f, u, firing.EventLoaderAssigned, nil, nil, &note)
	})
}

type loaderCandidateDTO struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	IsKilnMaster bool   `json:"is_kiln_master"`
	IsSelfLoader bool   `json:"is_self_loader"`
}

func (s *Server) handleLoaderCandidates(w http.ResponseWriter, r *http.Request) {
	if !userFromContext(r.Context()).Roles.CanManageFirings() {
		s.fail(w, r, forbidden("only kiln masters can do that"))
		return
	}
	rows, err := s.queries.ListLoaderCandidates(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]loaderCandidateDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, loaderCandidateDTO{row.ID, row.DisplayName, row.IsKilnMaster, row.IsSelfLoader})
	}
	writeJSON(w, http.StatusOK, out)
}
