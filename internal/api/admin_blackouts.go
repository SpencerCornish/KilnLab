package api

import (
	"context"
	"net/http"
	"time"

	"github.com/spencercornish/kilnlab/internal/db"
	"github.com/spencercornish/kilnlab/internal/firing"
)

// ── Kiln blackouts (admin-only availability blocks) ─────────────────────────

type blackoutDTO struct {
	ID        int64  `json:"id"`
	KilnID    int64  `json:"kiln_id"`
	StartDate Day    `json:"start_date"`
	EndDate   Day    `json:"end_date"`
	Reason    string `json:"reason"`
}

func toBlackoutDTO(b db.KilnBlackout) blackoutDTO {
	return blackoutDTO{ID: b.ID, KilnID: b.KilnID, StartDate: Day(b.StartDate), EndDate: Day(b.EndDate), Reason: b.Reason}
}

// blackoutForDay reports the blackout covering day for a kiln, if any.
func (s *Server) blackoutForDay(ctx context.Context, q *db.Queries, kilnID int64, day time.Time) (firing.Blackout, bool, error) {
	rows, err := q.ListBlackoutsForKiln(ctx, kilnID)
	if err != nil {
		return firing.Blackout{}, false, err
	}
	ranges := make([]firing.Blackout, len(rows))
	for i, b := range rows {
		ranges[i] = firing.Blackout{Start: b.StartDate, End: b.EndDate, Reason: b.Reason}
	}
	b, ok := firing.BlackoutForDay(day, ranges)
	return b, ok, nil
}

// requireAvailable rejects a day the kiln is blacked out. Blackouts are a hard
// block for everyone, including kiln masters; there is no override.
func (s *Server) requireAvailable(ctx context.Context, q *db.Queries, kilnID int64, day time.Time) error {
	b, blocked, err := s.blackoutForDay(ctx, q, kilnID, day)
	if err != nil {
		return err
	}
	if !blocked {
		return nil
	}
	if b.Reason != "" {
		return conflict("%s is unavailable (%s)", firing.DayLabel(day), b.Reason)
	}
	return conflict("%s is unavailable", firing.DayLabel(day))
}

func (s *Server) handleListBlackouts(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	rows, err := s.queries.ListBlackouts(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]blackoutDTO, 0, len(rows))
	for _, b := range rows {
		out = append(out, toBlackoutDTO(b))
	}
	writeJSON(w, http.StatusOK, out)
}

type blackoutInput struct {
	KilnID    int64  `json:"kiln_id"`
	StartDate Day    `json:"start_date"`
	EndDate   Day    `json:"end_date"`
	Reason    string `json:"reason"`
}

func (s *Server) handleCreateBlackout(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var in blackoutInput
	if !s.readJSON(w, r, &in) {
		return
	}
	start, end := time.Time(in.StartDate), time.Time(in.EndDate)
	if start.IsZero() || end.IsZero() {
		s.fail(w, r, badRequest("start and end dates are required"))
		return
	}
	if end.Before(start) {
		s.fail(w, r, badRequest("the end date must be on or after the start date"))
		return
	}
	reason, err := text("reason", in.Reason, maxReason, false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if in.KilnID == 0 {
		s.fail(w, r, badRequest("choose a kiln"))
		return
	}
	if _, err := s.queries.GetKiln(r.Context(), in.KilnID); err != nil {
		s.fail(w, r, err) // pgx.ErrNoRows → 404
		return
	}
	u := userFromContext(r.Context())
	b, err := s.queries.CreateBlackout(r.Context(), db.CreateBlackoutParams{
		KilnID: in.KilnID, StartDate: start, EndDate: end, Reason: reason, CreatedBy: &u.ID,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toBlackoutDTO(b))
}

func (s *Server) handleDeleteBlackout(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	n, err := s.queries.DeleteBlackout(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if n == 0 {
		s.fail(w, r, errNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
