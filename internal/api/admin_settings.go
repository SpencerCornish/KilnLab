package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/spencercornish/kilnlab/internal/db"
)

// ── /admin/settings (spec §13): everything in settings, plus kilns ─────────

type settingsDTO struct {
	Timezone                 string       `json:"timezone"`
	MinNoticeHours           int32        `json:"min_notice_hours"`
	RequestCutoffHours       int32        `json:"request_cutoff_hours"`
	UnderfilledThresholdPct  int32        `json:"underfilled_threshold_pct"`
	FiringTypes              []string     `json:"firing_types"`
	Cones                    []string     `json:"cones"`
	Programs                 []programDTO `json:"programs"`
	DiscordChannelID         string       `json:"discord_channel_id"`
	SlackChannelID           string       `json:"slack_channel_id"`
	MissedStartGraceMinutes  int32        `json:"missed_start_grace_minutes"`
	MissedUnloadGraceMinutes int32        `json:"missed_unload_grace_minutes"`
	PublicPageDaysAhead      int32        `json:"public_page_days_ahead"`
}

// adminSettingsDTO adds read-only context the page shows next to the form.
type adminSettingsDTO struct {
	Settings settingsDTO `json:"settings"`
	Kilns    []kilnDTO   `json:"kilns"`
	Env      struct {
		BaseURL                string `json:"base_url"` // for printable QR codes
		DiscordBot             bool   `json:"discord_bot"`
		SlackBot               bool   `json:"slack_bot"`
		DiscordChannelFallback string `json:"discord_channel_fallback"`
		SlackChannelFallback   string `json:"slack_channel_fallback"`
		DryRun                 bool   `json:"dry_run"`
	} `json:"env"`
}

func toSettingsDTO(st db.Setting) settingsDTO {
	return settingsDTO{
		Timezone: st.Timezone, MinNoticeHours: st.MinNoticeHours, RequestCutoffHours: st.RequestCutoffHours,
		UnderfilledThresholdPct: st.UnderfilledThresholdPct, FiringTypes: st.FiringTypes, Cones: st.Cones,
		Programs: parsePrograms(st.Programs), DiscordChannelID: st.DiscordChannelID, SlackChannelID: st.SlackChannelID,
		MissedStartGraceMinutes: st.MissedStartGraceMinutes, MissedUnloadGraceMinutes: st.MissedUnloadGraceMinutes,
		PublicPageDaysAhead: st.PublicPageDaysAhead,
	}
}

func intIn(field string, v, lo, hi int32) error {
	if v < lo || v > hi {
		return badRequest("%s must be between %d and %d", field, lo, hi)
	}
	return nil
}

// cleanList trims, drops blanks and rejects duplicates / overlong entries.
func cleanList(field string, in []string, maxItems, maxLen int) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if len([]rune(s)) > maxLen {
			return nil, badRequest("%s: %q is too long (max %d characters)", field, s, maxLen)
		}
		if slices.ContainsFunc(out, func(o string) bool { return strings.EqualFold(o, s) }) {
			return nil, badRequest("%s: %q is listed twice", field, s)
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, badRequest("%s needs at least one entry", field)
	}
	if len(out) > maxItems {
		return nil, badRequest("%s: at most %d entries", field, maxItems)
	}
	return out, nil
}

// validate checks an edited settings form and returns the update to store.
func (in settingsDTO) validate() (db.UpdateSettingsParams, error) {
	var p db.UpdateSettingsParams
	tz := strings.TrimSpace(in.Timezone)
	if _, err := time.LoadLocation(tz); err != nil || tz == "" || tz == "Local" {
		return p, badRequest("unknown timezone %q (use a name like America/Denver)", in.Timezone)
	}
	for _, err := range []error{
		intIn("Minimum notice", in.MinNoticeHours, 0, 24*30),
		intIn("Request cutoff", in.RequestCutoffHours, 0, 24*7),
		intIn("Under-filled threshold", in.UnderfilledThresholdPct, 0, 100),
		intIn("Missed start grace", in.MissedStartGraceMinutes, 0, 24*60),
		intIn("Missed unload grace", in.MissedUnloadGraceMinutes, 0, 24*60),
		intIn("Public page days ahead", in.PublicPageDaysAhead, 1, 365),
	} {
		if err != nil {
			return p, err // the first problem is enough for a form
		}
	}
	types, err := cleanList("Firing types", in.FiringTypes, 20, 30)
	if err != nil {
		return p, err
	}
	cones, err := cleanList("Cones", in.Cones, 40, 10)
	if err != nil {
		return p, err
	}
	programs := make([]programDTO, 0, len(in.Programs))
	for _, pr := range in.Programs {
		name := strings.TrimSpace(pr.Name)
		if name == "" {
			continue
		}
		if len([]rune(name)) > maxProgram {
			return p, badRequest("program name %q is too long", name)
		}
		if slices.ContainsFunc(programs, func(o programDTO) bool { return strings.EqualFold(o.Name, name) }) {
			return p, badRequest("program %q is listed twice", name)
		}
		if d := pr.DefaultDurationMinutes; d != nil && (*d < 1 || *d > 7*24*60) {
			return p, badRequest("program %q: duration must be between 1 minute and 7 days", name)
		}
		programs = append(programs, programDTO{Name: name, DefaultDurationMinutes: pr.DefaultDurationMinutes})
	}
	if len(programs) > 30 {
		return p, badRequest("at most 30 programs")
	}
	raw, _ := json.Marshal(programs)
	discord, slack := strings.TrimSpace(in.DiscordChannelID), strings.TrimSpace(in.SlackChannelID)
	if len(discord) > 100 || len(slack) > 100 {
		return p, badRequest("channel ids are too long")
	}
	return db.UpdateSettingsParams{
		Timezone: tz, MinNoticeHours: in.MinNoticeHours, RequestCutoffHours: in.RequestCutoffHours,
		UnderfilledThresholdPct: in.UnderfilledThresholdPct, FiringTypes: types, Cones: cones, Programs: raw,
		DiscordChannelID: discord, SlackChannelID: slack, MissedStartGraceMinutes: in.MissedStartGraceMinutes,
		MissedUnloadGraceMinutes: in.MissedUnloadGraceMinutes, PublicPageDaysAhead: in.PublicPageDaysAhead,
	}, nil
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !userFromContext(r.Context()).Roles.CanEditSettings() {
		s.fail(w, r, forbidden("only admins can change settings"))
		return false
	}
	return true
}

func (s *Server) adminSettings(r *http.Request, st db.Setting) (adminSettingsDTO, error) {
	var out adminSettingsDTO
	kilns, err := s.queries.ListKilns(r.Context())
	if err != nil {
		return out, err
	}
	out.Settings = toSettingsDTO(st)
	out.Kilns = make([]kilnDTO, 0, len(kilns))
	for _, k := range kilns {
		out.Kilns = append(out.Kilns, toKilnDTO(k))
	}
	out.Env.BaseURL = s.cfg.AppBaseURL
	out.Env.DiscordBot, out.Env.SlackBot = s.notifyCfg.DiscordBotToken != "", s.notifyCfg.SlackBotToken != ""
	out.Env.DiscordChannelFallback, out.Env.SlackChannelFallback = s.notifyCfg.DiscordChannelFallback, s.notifyCfg.SlackChannelFallback
	out.Env.DryRun = s.notifyCfg.DryRun
	return out, nil
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	st, err := s.queries.GetSettings(r.Context())
	if err == nil {
		var out adminSettingsDTO
		if out, err = s.adminSettings(r, st); err == nil {
			writeJSON(w, http.StatusOK, out)
			return
		}
	}
	s.fail(w, r, err)
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var in settingsDTO
	if !s.readJSON(w, r, &in) {
		return
	}
	p, err := in.validate()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	st, err := s.queries.UpdateSettings(r.Context(), p)
	if err == nil {
		var out adminSettingsDTO
		if out, err = s.adminSettings(r, st); err == nil {
			s.logger.Info("settings updated", "by", userFromContext(r.Context()).ID)
			writeJSON(w, http.StatusOK, out)
			return
		}
	}
	s.fail(w, r, err)
}

// ── Kilns ───────────────────────────────────────────────────────────────────

type kilnInput struct {
	Name             string   `json:"name"`
	Active           bool     `json:"active"`
	InteriorWidthIn  *float64 `json:"interior_width_in"`
	InteriorDepthIn  *float64 `json:"interior_depth_in"`
	InteriorHeightIn *float64 `json:"interior_height_in"`
	ShelfCount       *int32   `json:"shelf_count"`
	SharePerShelfPct *int32   `json:"share_per_shelf_pct"`
}

func (in kilnInput) validate() (db.CreateKilnParams, error) {
	name, err := text("Name", in.Name, 60, true)
	if err != nil {
		return db.CreateKilnParams{}, err
	}
	for _, d := range []*float64{in.InteriorWidthIn, in.InteriorDepthIn, in.InteriorHeightIn} {
		if d != nil && (*d <= 0 || *d > 200) {
			return db.CreateKilnParams{}, badRequest("interior dimensions must be between 0 and 200 inches")
		}
	}
	if in.ShelfCount != nil {
		if err := intIn("Shelf count", *in.ShelfCount, 1, 50); err != nil {
			return db.CreateKilnParams{}, err
		}
	}
	if in.SharePerShelfPct != nil {
		if err := intIn("Share per shelf", *in.SharePerShelfPct, 1, 100); err != nil {
			return db.CreateKilnParams{}, err
		}
	}
	return db.CreateKilnParams{
		Name: name, Active: in.Active, InteriorWidthIn: in.InteriorWidthIn, InteriorDepthIn: in.InteriorDepthIn,
		InteriorHeightIn: in.InteriorHeightIn, ShelfCount: in.ShelfCount, SharePerShelfPct: in.SharePerShelfPct,
	}, nil
}

func (s *Server) handleCreateKiln(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var in kilnInput
	if !s.readJSON(w, r, &in) {
		return
	}
	p, err := in.validate()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	k, err := s.queries.CreateKiln(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toKilnDTO(k))
}

func (s *Server) handleUpdateKiln(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in kilnInput
	if !s.readJSON(w, r, &in) {
		return
	}
	p, err := in.validate()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var k db.Kiln
	err = db.InTx(r.Context(), s.pool, func(q *db.Queries) error {
		var err error
		k, err = q.UpdateKiln(r.Context(), db.UpdateKilnParams{
			ID: id, Name: p.Name, Active: p.Active, InteriorWidthIn: p.InteriorWidthIn, InteriorDepthIn: p.InteriorDepthIn,
			InteriorHeightIn: p.InteriorHeightIn, ShelfCount: p.ShelfCount, SharePerShelfPct: p.SharePerShelfPct,
		})
		if err != nil {
			return err // no rows → 404
		}
		if n, err := q.CountActiveKilns(r.Context()); err != nil {
			return err
		} else if n == 0 {
			return conflict("keep at least one kiln active")
		}
		return nil
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toKilnDTO(k))
}
