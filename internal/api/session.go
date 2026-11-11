package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/spencercornish/kilnlab/internal/config"
	"github.com/spencercornish/kilnlab/internal/db"
	"github.com/spencercornish/kilnlab/internal/policy"
)

const csrfCookieName = "kilnlab_csrf"

type ctxKey int

const userKey ctxKey = iota

// currentUser is the signed-in user plus derived roles.
type currentUser struct {
	db.User
	Roles policy.Roles
}

func userFromContext(ctx context.Context) *currentUser {
	u, _ := ctx.Value(userKey).(*currentUser)
	return u
}

// loadSession attaches the signed-in user (if any) to the request context.
// The cookie holds a random token; the DB stores only its SHA-256.
func (s *Server) loadSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(config.SessionCookieName)
		if err != nil || c.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		u, err := s.queries.GetSessionUser(r.Context(), sha256Hex(c.Value))
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				s.writeError(w, r, http.StatusInternalServerError, "session check failed", err)
				return
			}
			s.clearCookie(w, config.SessionCookieName)
			next.ServeHTTP(w, r)
			return
		}
		roles, err := s.rolesFor(r.Context(), s.queries, u)
		if err != nil {
			s.writeError(w, r, http.StatusInternalServerError, "session check failed", err)
			return
		}
		ctx := context.WithValue(r.Context(), userKey, &currentUser{User: u, Roles: roles})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) rolesFor(ctx context.Context, q *db.Queries, u db.User) (policy.Roles, error) {
	km, err := q.UserIsKilnMaster(ctx, u.ID)
	if err != nil {
		return policy.Roles{}, err
	}
	return policy.Roles{SelfLoader: u.IsSelfLoader, KilnMaster: km, Admin: u.IsAdmin}, nil
}

func (s *Server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userFromContext(r.Context()) == nil {
			s.writeError(w, r, http.StatusUnauthorized, "sign in required", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// createSession stores a new session and sets the cookie.
func (s *Server) createSession(ctx context.Context, w http.ResponseWriter, q *db.Queries, userID int64) error {
	token := randomToken()
	if err := q.CreateSession(ctx, db.CreateSessionParams{
		ID:        sha256Hex(token),
		UserID:    userID,
		ExpiresAt: s.now().Add(config.SessionDuration),
	}); err != nil {
		return err
	}
	s.setCookie(w, config.SessionCookieName, token, config.SessionDuration, true)
	return nil
}

// setCookie writes a SameSite=Lax cookie. Lax (not Strict) is required so the
// session and OAuth binding cookies survive the top-level redirect back from
// Discord/Slack.
func (s *Server) setCookie(w http.ResponseWriter, name, value string, maxAge time.Duration, httpOnly bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: httpOnly,
		Secure:   s.cfg.SecureCookies(),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, Secure: s.cfg.SecureCookies(), SameSite: http.SameSiteLaxMode})
}

// csrfProtect enforces a double-submit token on mutating /api requests. The
// kilnlab_csrf cookie is readable by JS so the SPA can echo it in X-CSRF-Token.
func (s *Server) csrfProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := ""
		if c, err := r.Cookie(csrfCookieName); err == nil {
			token = c.Value
		}
		if token == "" {
			token = randomToken()
			s.setCookie(w, csrfCookieName, token, config.SessionDuration, false)
		}
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(token)) != 1 {
				s.writeError(w, r, http.StatusForbidden, "invalid csrf token", nil)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
