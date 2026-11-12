package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/spencercornish/kilnlab/internal/db"
	"github.com/spencercornish/kilnlab/internal/firing"
	"github.com/spencercornish/kilnlab/internal/notify"
)

// ── Errors ──────────────────────────────────────────────────────────────────

// apiError is a client-facing error with an HTTP status.
type apiError struct {
	status int
	msg    string
}

func (e *apiError) Error() string { return e.msg }

func badRequest(format string, args ...any) error {
	return &apiError{http.StatusBadRequest, fmt.Sprintf(format, args...)}
}
func conflict(format string, args ...any) error {
	return &apiError{http.StatusConflict, fmt.Sprintf(format, args...)}
}
func forbidden(msg string) error { return &apiError{http.StatusForbidden, msg} }

var errNotFound = &apiError{http.StatusNotFound, "not found"}

// fail maps domain/db errors to HTTP responses.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var ae *apiError
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &ae):
		s.writeError(w, r, ae.status, ae.msg, nil)
	case errors.Is(err, firing.ErrForbidden):
		s.writeError(w, r, http.StatusForbidden, strings.TrimPrefix(err.Error(), firing.ErrForbidden.Error()+": "), nil)
	case errors.Is(err, firing.ErrInvalidTransition):
		s.writeError(w, r, http.StatusConflict, strings.TrimPrefix(err.Error(), firing.ErrInvalidTransition.Error()+": "), nil)
	case errors.Is(err, pgx.ErrNoRows):
		s.writeError(w, r, http.StatusNotFound, "not found", nil)
	case errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "firing_requests_one_live_per_user_idx":
		s.writeError(w, r, http.StatusConflict, "you already have a request in this firing", nil)
	default:
		s.writeError(w, r, http.StatusInternalServerError, "something went wrong", err)
	}
}

func idParam(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		return 0, errNotFound
	}
	return id, nil
}

// ── Input validation ────────────────────────────────────────────────────────

const (
	maxTitle       = 120
	maxDescription = 1000
	maxNotes       = 2000
	maxClayBody    = 120
	maxReason      = 500
)

// text trims s and checks its length. required rejects empty.
func text(field, s string, maxLen int, required bool) (string, error) {
	s = strings.TrimSpace(s)
	if required && s == "" {
		return "", badRequest("%s is required", field)
	}
	if utf8.RuneCountInString(s) > maxLen {
		return "", badRequest("%s is too long (max %d characters)", field, maxLen)
	}
	return s, nil
}

// optText is text() returning nil for empty.
func optText(field string, s *string, maxLen int) (*string, error) {
	if s == nil {
		return nil, nil
	}
	v, err := text(field, *s, maxLen, false)
	if err != nil || v == "" {
		return nil, err
	}
	return &v, nil
}

func checkShare(pct int32) error {
	if pct < 1 || pct > 100 {
		return badRequest("share must be between 1 and 100%%")
	}
	return nil
}

func checkCone(settings db.Setting, cone string) error {
	if !slices.Contains(settings.Cones, cone) {
		return badRequest("unknown cone %q", cone)
	}
	return nil
}

// ── Response shapes ─────────────────────────────────────────────────────────

type userRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type firingDTO struct {
	ID                       int64           `json:"id"`
	KilnID                   int64           `json:"kiln_id"`
	Title                    *string         `json:"title"`
	FiringType               string          `json:"firing_type"`
	Cone                     string          `json:"cone"`
	Notes                    string          `json:"notes"`
	ScheduledDate            Day             `json:"scheduled_date"`
	Status                   db.FiringStatus `json:"status"`
	HoldReason               *string         `json:"hold_reason"`
	CancelReason             *string         `json:"cancel_reason"`
	Proposer                 userRef         `json:"proposer"`
	ProposerDescription      string          `json:"proposer_description"`
	ProposerSharePct         int32           `json:"proposer_share_pct"`
	ProposerCone             string          `json:"proposer_cone"`
	ProposerClayBody         *string         `json:"proposer_clay_body"`
	Fullness                 firing.Fullness `json:"fullness"`
	RequestsCloseAt          time.Time       `json:"requests_close_at"`
	RequestsClosedManually   bool            `json:"requests_closed_manually"`
	RequestsOpen             bool            `json:"requests_open"`
	LoaderUserID             *int64          `json:"loader_user_id"`
	Program                  *string         `json:"program"`
	EstimatedDurationMinutes *int32          `json:"estimated_duration_minutes"`
	StartedAt                *time.Time      `json:"started_at"`
	EstimatedUnloadAt        *time.Time      `json:"estimated_unload_at"`
	CoolingAt                *time.Time      `json:"cooling_at"`
	UnloadedAt               *time.Time      `json:"unloaded_at"`
	ReadyForPickupAt         *time.Time      `json:"ready_for_pickup_at"`
	CompletedAt              *time.Time      `json:"completed_at"`
	CreatedAt                time.Time       `json:"created_at"`
}

func (s *Server) toFiringDTO(f db.Firing, proposerName string, approved []int32) firingDTO {
	return firingDTO{
		ID: f.ID, KilnID: f.KilnID, Title: f.Title, FiringType: f.FiringType, Cone: f.Cone, Notes: f.Notes,
		ScheduledDate: Day(f.ScheduledDate), Status: f.Status, HoldReason: f.HoldReason, CancelReason: f.CancelReason,
		Proposer:            userRef{f.ProposedByUserID, proposerName},
		ProposerDescription: f.ProposerDescription, ProposerSharePct: f.ProposerSharePct,
		ProposerCone: f.ProposerCone, ProposerClayBody: f.ProposerClayBody,
		Fullness:        firing.ComputeFullness(f.ProposerSharePct, approved, f.FullnessOverridePct),
		RequestsCloseAt: f.RequestsCloseAt, RequestsClosedManually: f.RequestsClosedManually,
		RequestsOpen: firing.RequestsOpen(window(f), s.now()),
		Program:      f.Program, EstimatedDurationMinutes: f.EstimatedDurationMinutes,
		StartedAt: f.StartedAt, EstimatedUnloadAt: f.EstimatedUnloadAt, CoolingAt: f.CoolingAt,
		UnloadedAt: f.UnloadedAt, ReadyForPickupAt: f.ReadyForPickupAt, CompletedAt: f.CompletedAt,
		CreatedAt: f.CreatedAt,
	}
}

func window(f db.Firing) firing.Window {
	return firing.Window{Status: f.Status, CloseAt: f.RequestsCloseAt, ClosedManually: f.RequestsClosedManually}
}

type firingSummaryDTO struct {
	firingDTO
	ApprovedCount int32 `json:"approved_count"`
	PendingCount  int32 `json:"pending_count"`
	IsMine        bool  `json:"is_mine"` // I proposed it or have a live request in it
}

type requestDTO struct {
	ID                   int64            `json:"id"`
	User                 userRef          `json:"user"`
	UserAvatarURL        *string          `json:"user_avatar_url"`
	UserCommunity        db.Community     `json:"user_community"`
	Description          string           `json:"description"`
	EstimatedSharePct    int32            `json:"estimated_share_pct"`
	Cone                 string           `json:"cone"`
	ClayBody             *string          `json:"clay_body"`
	Glazed               bool             `json:"glazed"`
	Status               db.RequestStatus `json:"status"`
	DecisionNote         *string          `json:"decision_note,omitempty"`
	DecidedAt            *time.Time       `json:"decided_at"`
	CompatibilityWarning *string          `json:"compatibility_warning,omitempty"`
	CreatedAt            time.Time        `json:"created_at"`
	Can                  requestPermsDTO  `json:"can"`
}

type requestPermsDTO struct {
	Edit     bool `json:"edit"`
	Withdraw bool `json:"withdraw"`
	Decide   bool `json:"decide"`
}

type eventDTO struct {
	ID         int64            `json:"id"`
	Type       string           `json:"type"`
	Actor      *userRef         `json:"actor"`
	FromStatus *db.FiringStatus `json:"from_status"`
	ToStatus   *db.FiringStatus `json:"to_status"`
	OccurredAt time.Time        `json:"occurred_at"`
	Note       *string          `json:"note"`
	Source     db.EventSource   `json:"source"`
}

// firingPermsDTO tells the SPA which controls to show. The server re-checks
// every action anyway.
type firingPermsDTO struct {
	Request    bool `json:"request"`
	Manage     bool `json:"manage"` // override fullness, close/reopen requests
	Confirm    bool `json:"confirm"`
	Hold       bool `json:"hold"`
	Release    bool `json:"release"`
	Reschedule bool `json:"reschedule"`
	Cancel     bool `json:"cancel"`

	// Lifecycle (M2).
	NextSteps         []db.FiringStatus `json:"next_steps"`     // buttons to offer, in order
	BlockedReason     string            `json:"blocked_reason"` // why there are none, if relevant
	Backdate          bool              `json:"backdate"`       // may set an earlier time on a step
	CorrectTimestamps bool              `json:"correct_timestamps"`
}

type firingDetailDTO struct {
	Firing    firingDTO      `json:"firing"`
	Requests  []requestDTO   `json:"requests"`
	MyRequest *requestDTO    `json:"my_request"`
	Events    []eventDTO     `json:"events"`
	Can       firingPermsDTO `json:"can"`
}

// firingDetail loads everything the firing page shows, filtered for u.
func (s *Server) firingDetail(ctx context.Context, q *db.Queries, id int64, u *currentUser) (*firingDetailDTO, error) {
	f, err := q.GetFiring(ctx, id)
	if err != nil {
		return nil, err
	}
	proposerName, err := q.GetUserName(ctx, f.ProposedByUserID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListRequestsForFiring(ctx, id)
	if err != nil {
		return nil, err
	}
	events, err := q.ListEventsForFiring(ctx, id)
	if err != nil {
		return nil, err
	}

	var approved []int32
	for _, row := range rows {
		if row.FiringRequest.Status == db.RequestStatusApproved {
			approved = append(approved, row.FiringRequest.EstimatedSharePct)
		}
	}
	d := &firingDetailDTO{Firing: s.toFiringDTO(f, proposerName, approved), Requests: []requestDTO{}, Events: []eventDTO{}}

	open := d.Firing.RequestsOpen
	canDecide := u.Roles.CanDecideRequests(f.ProposedByUserID == u.ID) && firing.CanDecideRequests(f.Status)
	seeDetails := u.Roles.CanSeeRequestDetails()
	hasLive, myIdx := false, -1
	for _, row := range rows {
		rq := row.FiringRequest
		mine := rq.UserID == u.ID
		live := rq.Status == db.RequestStatusPending || rq.Status == db.RequestStatusApproved
		if mine && live {
			hasLive = true
		}
		// Members see live requests plus their own history; kiln masters see everything.
		if !seeDetails && !mine && !live {
			continue
		}
		dto := requestDTO{
			ID: rq.ID, User: userRef{rq.UserID, row.UserName}, UserAvatarURL: row.UserAvatarUrl,
			UserCommunity: row.UserCommunity, Description: rq.Description, EstimatedSharePct: rq.EstimatedSharePct,
			Cone: rq.Cone, ClayBody: rq.ClayBody, Glazed: rq.Glazed, Status: rq.Status, DecidedAt: rq.DecidedAt,
			CreatedAt: rq.CreatedAt,
			Can: requestPermsDTO{
				Edit:     mine && live && open,
				Withdraw: mine && live && open,
				Decide:   canDecide && rq.Status != db.RequestStatusWithdrawn,
			},
		}
		if mine || seeDetails {
			dto.DecisionNote = rq.DecisionNote
			dto.CompatibilityWarning = rq.CompatibilityWarning
		}
		if mine && live {
			myIdx = len(d.Requests)
		}
		d.Requests = append(d.Requests, dto)
	}
	if myIdx >= 0 {
		d.MyRequest = &d.Requests[myIdx]
	}

	for _, e := range events {
		dto := eventDTO{ID: e.ID, Type: e.Type, FromStatus: e.FromStatus, ToStatus: e.ToStatus,
			OccurredAt: e.OccurredAt, Note: e.Note, Source: e.Source}
		if e.ActorUserID != nil && e.ActorName != nil {
			dto.Actor = &userRef{*e.ActorUserID, *e.ActorName}
		}
		d.Events = append(d.Events, dto)
	}

	actor := firing.Actor{Roles: u.Roles}
	st := firing.State{Status: f.Status, WasConfirmed: f.WasConfirmed}
	ok := func(_ firing.Status, err error) bool { return err == nil }
	d.Can = firingPermsDTO{
		Request:    open && !hasLive && f.ProposedByUserID != u.ID,
		Manage:     u.Roles.CanManageFirings() && firing.IsActive(f.Status),
		Confirm:    ok(firing.Confirm(st, actor)),
		Hold:       ok(firing.Hold(st, actor)),
		Release:    ok(firing.Release(st, actor)),
		Reschedule: firing.Reschedule(st, actor) == nil,
		Cancel:     ok(firing.Cancel(st, actor)),

		NextSteps:         firing.NextSteps(st, actor),
		BlockedReason:     firing.BlockedReason(st, actor),
		Backdate:          u.Roles.CanCorrectTimestamps(),
		CorrectTimestamps: u.Roles.CanCorrectTimestamps(),
	}
	if d.Can.NextSteps == nil {
		d.Can.NextSteps = []db.FiringStatus{}
	}
	return d, nil
}

// ── Handlers ────────────────────────────────────────────────────────────────

type kilnDTO struct {
	ID               int64    `json:"id"`
	Name             string   `json:"name"`
	Active           bool     `json:"active"`
	InteriorWidthIn  *float64 `json:"interior_width_in"`
	InteriorDepthIn  *float64 `json:"interior_depth_in"`
	InteriorHeightIn *float64 `json:"interior_height_in"`
	ShelfCount       *int32   `json:"shelf_count"`
	SharePerShelfPct *int32   `json:"share_per_shelf_pct"`
}

func toKilnDTO(k db.Kiln) kilnDTO {
	return kilnDTO{ID: k.ID, Name: k.Name, Active: k.Active, InteriorWidthIn: k.InteriorWidthIn,
		InteriorDepthIn: k.InteriorDepthIn, InteriorHeightIn: k.InteriorHeightIn, ShelfCount: k.ShelfCount,
		SharePerShelfPct: k.SharePerShelfPct}
}

type optionsDTO struct {
	Timezone           string       `json:"timezone"`
	MinNoticeHours     int32        `json:"min_notice_hours"`
	RequestCutoffHours int32        `json:"request_cutoff_hours"`
	FiringTypes        []string     `json:"firing_types"`
	Cones              []string     `json:"cones"`
	Kilns              []kilnDTO    `json:"kilns"` // active first; inactive kept for old firings
	Programs           []programDTO `json:"programs"`
}

// handleOptions returns the settings the propose/request forms need.
func (s *Server) handleOptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, err := s.queries.GetSettings(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	kilns, err := s.queries.ListKilns(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := optionsDTO{
		Timezone: st.Timezone, MinNoticeHours: st.MinNoticeHours, RequestCutoffHours: st.RequestCutoffHours,
		FiringTypes: st.FiringTypes, Cones: st.Cones, Kilns: make([]kilnDTO, 0, len(kilns)),
		Programs: parsePrograms(st.Programs),
	}
	for _, k := range kilns {
		out.Kilns = append(out.Kilns, toKilnDTO(k))
	}
	writeJSON(w, http.StatusOK, out)
}

// pickKiln resolves the kiln for a new firing. With a single active kiln the
// id may be omitted; with several it must be given.
func pickKiln(ctx context.Context, q *db.Queries, id int64) (db.Kiln, error) {
	if id != 0 {
		k, err := q.GetKiln(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !k.Active) {
			return k, badRequest("unknown kiln")
		}
		return k, err
	}
	kilns, err := q.ListKilns(ctx)
	if err != nil {
		return db.Kiln{}, err
	}
	var active []db.Kiln
	for _, k := range kilns {
		if k.Active {
			active = append(active, k)
		}
	}
	switch len(active) {
	case 0:
		return db.Kiln{}, conflict("there are no active kilns")
	case 1:
		return active[0], nil
	default:
		return db.Kiln{}, badRequest("choose a kiln")
	}
}

// recentFinishedWindow: completed/cancelled firings stay listed this long after their start.
const recentFinishedWindow = 3 * 24 * time.Hour

func (s *Server) handleListFirings(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	rows, err := s.queries.ListFiringSummaries(r.Context(), db.ListFiringSummariesParams{
		UserID: u.ID, Since: s.now().Add(-recentFinishedWindow),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]firingSummaryDTO, 0, len(rows))
	for _, row := range rows {
		approved := []int32{row.ApprovedSharePct} // the sum is all fullness needs
		out = append(out, firingSummaryDTO{
			firingDTO:     s.toFiringDTO(row.Firing, row.ProposerName, approved),
			ApprovedCount: row.ApprovedCount, PendingCount: row.PendingCount,
			IsMine: row.Firing.ProposedByUserID == u.ID || row.HasMyRequest,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetFiring(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err == nil {
		var d *firingDetailDTO
		d, err = s.firingDetail(r.Context(), s.queries, id, userFromContext(r.Context()))
		if err == nil {
			writeJSON(w, http.StatusOK, d)
			return
		}
	}
	s.fail(w, r, err)
}

type createFiringRequest struct {
	KilnID        int64   `json:"kiln_id"` // optional while only one kiln is active
	FiringType    string  `json:"firing_type"`
	Cone          string  `json:"cone"`
	Title         *string `json:"title"`
	Notes         string  `json:"notes"`
	ScheduledDate Day     `json:"scheduled_date"` // the day; no start time is promised
	Description   string  `json:"description"`
	SharePct      int32   `json:"share_pct"`
	ProposerCone  string  `json:"proposer_cone"` // defaults to cone
	ClayBody      *string `json:"clay_body"`
}

func (s *Server) handleCreateFiring(w http.ResponseWriter, r *http.Request) {
	var req createFiringRequest
	if !s.readJSON(w, r, &req) {
		return
	}
	u := userFromContext(r.Context())
	var id int64
	err := db.InTx(r.Context(), s.pool, func(q *db.Queries) error {
		ctx := r.Context()
		st, err := q.GetSettings(ctx)
		if err != nil {
			return err
		}
		kiln, err := pickKiln(ctx, q, req.KilnID)
		if err != nil {
			return err
		}
		p := db.CreateFiringParams{KilnID: kiln.ID, ProposedByUserID: u.ID, ProposerSharePct: req.SharePct}
		if !slices.Contains(st.FiringTypes, req.FiringType) {
			return badRequest("unknown firing type %q", req.FiringType)
		}
		p.FiringType = req.FiringType
		if err := checkCone(st, req.Cone); err != nil {
			return err
		}
		p.Cone = req.Cone
		p.ProposerCone = req.Cone
		if req.ProposerCone != "" {
			if err := checkCone(st, req.ProposerCone); err != nil {
				return err
			}
			p.ProposerCone = req.ProposerCone
		}
		day := time.Time(req.ScheduledDate)
		if day.IsZero() {
			return badRequest("date is required")
		}
		loc := s.studioLocation(ctx, q)
		if !firing.DayEnd(day, loc).After(s.now()) {
			return badRequest("that day has already passed")
		}
		p.ScheduledDate = day
		start := firing.DayStart(day, loc)
		if !u.Roles.CanBypassMinNotice() {
			if err := firing.CheckMinNotice(start, s.now(), st.MinNoticeHours); err != nil {
				return badRequest("%s", err.Error())
			}
		}
		p.RequestsCloseAt = firing.RequestsCloseAt(start, st.RequestCutoffHours)
		if err := checkShare(req.SharePct); err != nil {
			return err
		}
		if p.ProposerDescription, err = text("description", req.Description, maxDescription, true); err != nil {
			return err
		}
		if p.Notes, err = text("notes", req.Notes, maxNotes, false); err != nil {
			return err
		}
		if p.Title, err = optText("title", req.Title, maxTitle); err != nil {
			return err
		}
		if p.ProposerClayBody, err = optText("clay body", req.ClayBody, maxClayBody); err != nil {
			return err
		}

		f, err := q.CreateFiring(ctx, p)
		if err != nil {
			return err
		}
		id = f.ID
		if err := s.logEvent(ctx, q, f, u, firing.EventCreated, nil, &f.Status, nil); err != nil {
			return err
		}
		return notify.Enqueue(ctx, q, s.notifyCfg, f.ID, u.ID, &notify.Batch{})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.notifier.Kick()
	d, err := s.firingDetail(r.Context(), s.queries, id, u)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

// logEvent writes a firing_events row stamped now, from the web UI.
func (s *Server) logEvent(ctx context.Context, q *db.Queries, f db.Firing, u *currentUser, typ string, from, to *db.FiringStatus, note *string) error {
	return q.InsertEvent(ctx, db.InsertEventParams{
		FiringID: f.ID, ActorUserID: &u.ID, Type: typ, FromStatus: from, ToStatus: to,
		OccurredAt: s.now(), Note: note, Source: db.EventSourceWeb,
	})
}
