package api

import (
	"context"
	"net/http"

	"github.com/spencercornish/kilnlab/internal/db"
)

// ── Admin: firing profiles (per kiln) ───────────────────────────────────────

type adminProfileInput struct {
	KilnID          int64  `json:"kiln_id"`
	Name            string `json:"name"`
	FiringType      string `json:"firing_type"`
	Cone            string `json:"cone"`
	FiringMinutes   int32  `json:"firing_minutes"`
	CooldownMinutes int32  `json:"cooldown_minutes"`
	Active          bool   `json:"active"`
}

func (in adminProfileInput) validate(ctx context.Context, q *db.Queries) (profileSnapshot, error) {
	st, err := q.GetSettings(ctx)
	if err != nil {
		return profileSnapshot{}, err
	}
	return validateProfileValues(st, profileValues{
		Name: in.Name, FiringType: in.FiringType, Cone: in.Cone,
		FiringMinutes: in.FiringMinutes, CooldownMinutes: in.CooldownMinutes,
	})
}

func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	ps, err := s.queries.ListProfiles(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, profileDTOs(ps))
}

func (s *Server) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var in adminProfileInput
	if !s.readJSON(w, r, &in) {
		return
	}
	if _, err := s.queries.GetKiln(r.Context(), in.KilnID); err != nil {
		s.fail(w, r, err)
		return
	}
	snap, err := in.validate(r.Context(), s.queries)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p, err := s.queries.CreateProfile(r.Context(), db.CreateProfileParams{
		KilnID: in.KilnID, Name: snap.Title, FiringType: snap.FiringType, Cone: snap.Cone,
		FiringMinutes: snap.FiringMinutes, CooldownMinutes: snap.CooldownMinutes, Active: in.Active,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toProfileDTO(p))
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in adminProfileInput
	if !s.readJSON(w, r, &in) {
		return
	}
	if _, err := s.queries.GetKiln(r.Context(), in.KilnID); err != nil {
		s.fail(w, r, err)
		return
	}
	snap, err := in.validate(r.Context(), s.queries)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p, err := s.queries.UpdateProfile(r.Context(), db.UpdateProfileParams{
		ID: id, KilnID: in.KilnID, Name: snap.Title, FiringType: snap.FiringType, Cone: snap.Cone,
		FiringMinutes: snap.FiringMinutes, CooldownMinutes: snap.CooldownMinutes, Active: in.Active,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toProfileDTO(p))
}

func (s *Server) handleDeactivateProfile(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p, err := s.queries.DeactivateProfile(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toProfileDTO(p))
}
