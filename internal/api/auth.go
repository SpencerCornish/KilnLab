package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/spencercornish/kilnlab/internal/auth"
	"github.com/spencercornish/kilnlab/internal/config"
	"github.com/spencercornish/kilnlab/internal/db"
)

const (
	oauthBindingCookie = "kilnlab_oauth"
	oauthStateTTL      = 15 * time.Minute
)

// safeReturnTo accepts only same-origin relative paths, so the login flow
// cannot be used as an open redirect.
func safeReturnTo(raw string) string {
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.ContainsAny(raw, "\\\r\n") {
		return "/"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || strings.HasPrefix(u.Path, "/api/") {
		return "/"
	}
	return raw
}

func (s *Server) loginErrorRedirect(w http.ResponseWriter, r *http.Request, code, returnTo string) {
	q := url.Values{"error": {code}}
	if returnTo != "" && returnTo != "/" {
		q.Set("return_to", returnTo)
	}
	http.Redirect(w, r, "/login?"+q.Encode(), http.StatusSeeOther)
}

func (s *Server) handleAuthProviders(w http.ResponseWriter, r *http.Request) {
	_, discord := s.providers["discord"]
	_, slack := s.providers["slack"]
	writeJSON(w, http.StatusOK, map[string]bool{"discord": discord, "slack": slack, "dev": s.cfg.DevLogin})
}

// handleAuthStart begins OAuth. State lives in the DB so the callback still
// works when a phone returns from the Discord/Slack app into a different
// browser. A per-browser binding cookie is checked when it is present.
func (s *Server) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	p, ok := s.providers[chi.URLParam(r, "provider")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	returnTo := safeReturnTo(r.URL.Query().Get("return_to"))

	binding := ""
	if c, err := r.Cookie(oauthBindingCookie); err == nil && len(c.Value) == 64 {
		binding = c.Value
	} else {
		binding = randomToken()
	}
	s.setCookie(w, oauthBindingCookie, binding, oauthStateTTL, true)

	state := randomToken()
	if err := s.queries.CreateOAuthState(r.Context(), db.CreateOAuthStateParams{
		State:       state,
		Provider:    db.AuthProvider(p.Name()),
		ReturnTo:    returnTo,
		BrowserHash: sha256Hex(binding),
		ExpiresAt:   s.now().Add(oauthStateTTL),
	}); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "could not start sign in", err)
		return
	}
	http.Redirect(w, r, p.AuthCodeURL(state), http.StatusFound)
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	p, ok := s.providers[chi.URLParam(r, "provider")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()

	st, err := s.queries.ConsumeOAuthState(r.Context(), db.ConsumeOAuthStateParams{
		State:    q.Get("state"),
		Provider: db.AuthProvider(p.Name()),
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			s.logger.Error("oauth state lookup failed", "error", err)
		}
		s.loginErrorRedirect(w, r, "expired", "")
		return
	}
	if c, err := r.Cookie(oauthBindingCookie); err == nil {
		if sha256Hex(c.Value) != st.BrowserHash {
			// Same browser, different attempt: someone else's state. Refuse.
			s.logger.Warn("oauth binding mismatch", "provider", p.Name())
			s.loginErrorRedirect(w, r, "expired", st.ReturnTo)
			return
		}
		s.clearCookie(w, oauthBindingCookie)
	} else {
		s.logger.Info("oauth callback without binding cookie (likely returned in another browser)", "provider", p.Name())
	}

	if e := q.Get("error"); e != "" {
		s.logger.Info("oauth provider returned error", "provider", p.Name(), "error", e)
		s.loginErrorRedirect(w, r, "cancelled", st.ReturnTo)
		return
	}

	ident, err := p.Exchange(r.Context(), q.Get("code"))
	if err != nil {
		var rej *auth.RejectedError
		if errors.As(err, &rej) {
			s.logger.Info("login rejected", "provider", p.Name(), "code", rej.Code)
			s.loginErrorRedirect(w, r, rej.Code, st.ReturnTo)
			return
		}
		s.logger.Error("oauth exchange failed", "provider", p.Name(), "error", err)
		s.loginErrorRedirect(w, r, "failed", st.ReturnTo)
		return
	}

	if err := s.signIn(r.Context(), w, ident); err != nil {
		s.logger.Error("sign in failed", "provider", p.Name(), "error", err)
		s.loginErrorRedirect(w, r, "failed", st.ReturnTo)
		return
	}
	http.Redirect(w, r, st.ReturnTo, http.StatusSeeOther)
}

// signIn upserts the user and account from a verified identity, re-deriving
// roles, and starts a session.
func (s *Server) signIn(ctx context.Context, w http.ResponseWriter, ident *auth.Identity) error {
	grantAdmin := s.cfg.IsBootstrapAdmin(ident.Provider, ident.ProviderUserID)
	var avatar *string
	if ident.AvatarURL != "" {
		avatar = &ident.AvatarURL
	}
	return db.InTx(ctx, s.pool, func(q *db.Queries) error {
		acct, err := q.GetAccountByProvider(ctx, db.GetAccountByProviderParams{
			Provider:       db.AuthProvider(ident.Provider),
			ProviderUserID: ident.ProviderUserID,
		})
		var userID int64
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			u, err := q.CreateUser(ctx, db.CreateUserParams{
				DisplayName: ident.DisplayName,
				AvatarUrl:   avatar,
				Community:   db.Community(ident.Community),
				IsAdmin:     grantAdmin,
			})
			if err != nil {
				return err
			}
			if _, err := q.CreateAccount(ctx, db.CreateAccountParams{
				UserID:                u.ID,
				Provider:              db.AuthProvider(ident.Provider),
				ProviderUserID:        ident.ProviderUserID,
				ProviderTeamOrGuildID: ident.TeamOrGuildID,
				IsKilnMaster:          ident.IsKilnMaster,
			}); err != nil {
				return err
			}
			userID = u.ID
		case err != nil:
			return err
		default:
			if _, err := q.UpdateUserOnLogin(ctx, db.UpdateUserOnLoginParams{
				ID:          acct.UserID,
				DisplayName: ident.DisplayName,
				AvatarUrl:   avatar,
				GrantAdmin:  grantAdmin,
			}); err != nil {
				return err
			}
			if err := q.UpdateAccountOnLogin(ctx, db.UpdateAccountOnLoginParams{
				ID:                    acct.ID,
				ProviderTeamOrGuildID: ident.TeamOrGuildID,
				IsKilnMaster:          ident.IsKilnMaster,
			}); err != nil {
				return err
			}
			userID = acct.UserID
		}
		return s.createSession(ctx, w, q, userID)
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(config.SessionCookieName); err == nil && c.Value != "" {
		if err := s.queries.DeleteSession(r.Context(), sha256Hex(c.Value)); err != nil {
			s.writeError(w, r, http.StatusInternalServerError, "sign out failed", err)
			return
		}
	}
	s.clearCookie(w, config.SessionCookieName)
	w.WriteHeader(http.StatusNoContent)
}

type meResponse struct {
	ID          int64   `json:"id"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
	Community   string  `json:"community"`
	Roles       any     `json:"roles"`
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	writeJSON(w, http.StatusOK, meResponse{
		ID: u.ID, DisplayName: u.DisplayName, AvatarURL: u.AvatarUrl,
		Community: string(u.Community), Roles: u.Roles,
	})
}

type devLoginRequest struct {
	Name       string `json:"name"`
	Community  string `json:"community"`
	KilnMaster bool   `json:"kiln_master"`
	SelfLoader bool   `json:"self_loader"`
	Admin      bool   `json:"admin"`
}

// handleDevLogin signs in as a fake user (DEV_LOGIN only; refused on https).
// The same name always maps to the same user, so roles can be toggled per login.
func (s *Server) handleDevLogin(w http.ResponseWriter, r *http.Request) {
	var req devLoginRequest
	if !s.readJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 60 {
		s.writeError(w, r, http.StatusBadRequest, "name is required (max 60 characters)", nil)
		return
	}
	provider, team := "discord", s.cfg.ArtLabGuildID
	if req.Community == "bms" {
		provider, team = "slack", s.cfg.SlackTeamID
	} else {
		req.Community = "artlab"
	}
	if req.KilnMaster && provider != "discord" {
		s.writeError(w, r, http.StatusBadRequest, "kiln masters sign in with Discord", nil)
		return
	}
	ident := &auth.Identity{
		Provider:       provider,
		ProviderUserID: "dev-" + strings.ToLower(req.Name),
		TeamOrGuildID:  team,
		Community:      req.Community,
		DisplayName:    req.Name,
		IsKilnMaster:   req.KilnMaster,
	}
	err := s.signIn(r.Context(), w, ident)
	if err == nil {
		// Dev login can also set the flags normally managed in-app.
		_, err = s.pool.Exec(r.Context(),
			`UPDATE users SET is_self_loader = $2, is_admin = $3
			 WHERE id = (SELECT user_id FROM accounts WHERE provider = $1::auth_provider AND provider_user_id = $4)`,
			provider, req.SelfLoader, req.Admin, ident.ProviderUserID)
	}
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "dev login failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
