package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/spencercornish/kilnlab/internal/auth"
	"github.com/spencercornish/kilnlab/internal/config"
	"github.com/spencercornish/kilnlab/internal/db"
)

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run integration tests (e.g. postgres://kilnlab:kilnlab@localhost:5433/kilnlab_test?sslmode=disable)")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}
	t.Cleanup(pool.Close)
	// Truncating users cascades to firings and everything under them.
	if _, err := pool.Exec(ctx, `TRUNCATE users, accounts, sessions, oauth_states, notification_outbox RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	// Back to the single kiln seeded by the first migration.
	if _, err := pool.Exec(ctx, `DELETE FROM kilns WHERE id <> (SELECT min(id) FROM kilns)`); err != nil {
		t.Fatalf("reset kilns: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE kilns SET name = 'Studio kiln', active = true, interior_width_in = NULL,
		interior_depth_in = NULL, interior_height_in = NULL, shelf_count = NULL, share_per_shelf_pct = NULL;
		DELETE FROM settings; INSERT INTO settings (id) VALUES (1)`); err != nil {
		t.Fatalf("reset settings: %v", err)
	}
	return pool
}

// stubProvider skips the real OAuth server: AuthCodeURL points straight back at
// our callback, and Exchange returns a fixed identity (or rejection).
type stubProvider struct {
	name     string
	callback string
	ident    *auth.Identity
	reject   string
}

func (p *stubProvider) Name() string { return p.name }
func (p *stubProvider) AuthCodeURL(state string) string {
	return p.callback + "?code=c&state=" + url.QueryEscape(state)
}
func (p *stubProvider) Exchange(context.Context, string) (*auth.Identity, error) {
	if p.reject != "" {
		return nil, &auth.RejectedError{Code: p.reject}
	}
	id := *p.ident
	return &id, nil
}

type testEnv struct {
	srv     *httptest.Server
	discord *stubProvider
	pool    *pgxpool.Pool
}

func newTestEnv(t *testing.T) *testEnv {
	pool := openTestPool(t)
	env := &testEnv{pool: pool, discord: &stubProvider{name: "discord"}}
	cfg := &config.Config{AppBaseURL: "http://localhost", AdminUserIDs: "discord:owner"}
	s := New(cfg, pool, map[string]auth.Provider{"discord": env.discord}, nil, slog.New(slog.DiscardHandler))
	env.srv = httptest.NewServer(s.Handler())
	t.Cleanup(env.srv.Close)
	env.discord.callback = env.srv.URL + "/api/auth/discord/callback"
	return env
}

func newClient(t *testing.T) *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func getMe(t *testing.T, c *http.Client, base string) (int, map[string]any) {
	t.Helper()
	resp, err := c.Get(base + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body
}

func TestOAuthLoginFlow(t *testing.T) {
	env := newTestEnv(t)
	env.discord.ident = &auth.Identity{
		Provider: "discord", ProviderUserID: "owner", TeamOrGuildID: "g", Community: "artlab",
		DisplayName: "Cody", IsKilnMaster: true,
	}
	c := newClient(t)

	if code, _ := getMe(t, c, env.srv.URL); code != http.StatusUnauthorized {
		t.Fatalf("/me before login = %d, want 401", code)
	}

	resp, err := c.Get(env.srv.URL + "/api/auth/discord/start?return_to=/kiln")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	// Redirects are followed: start → stub provider → callback → /kiln (no SPA → 503).
	if resp.Request.URL.Path != "/kiln" {
		t.Fatalf("landed on %s, want /kiln", resp.Request.URL)
	}

	code, me := getMe(t, c, env.srv.URL)
	if code != http.StatusOK || me["display_name"] != "Cody" {
		t.Fatalf("/me = %d %v", code, me)
	}
	roles := me["roles"].(map[string]any)
	if roles["kiln_master"] != true || roles["admin"] != true {
		t.Fatalf("roles = %v, want kiln master + bootstrap admin", roles)
	}

	// Second login after losing the Discord role: kiln master is re-derived, admin is kept.
	env.discord.ident.IsKilnMaster = false
	env.discord.ident.DisplayName = "Cody K"
	c2 := newClient(t)
	resp, err = c2.Get(env.srv.URL + "/api/auth/discord/start")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	_, me = getMe(t, c2, env.srv.URL)
	roles = me["roles"].(map[string]any)
	if me["display_name"] != "Cody K" || roles["kiln_master"] != false || roles["admin"] != true {
		t.Fatalf("after re-login: %v", me)
	}
	var users int
	_ = env.pool.QueryRow(context.Background(), `SELECT count(*) FROM users`).Scan(&users)
	if users != 1 {
		t.Fatalf("users = %d, want 1 (same account reused)", users)
	}
}

func TestOAuthCallbackFromAnotherBrowser(t *testing.T) {
	// Phones can come back from the Discord app into a different browser that
	// lacks our binding cookie. Login should still succeed there.
	env := newTestEnv(t)
	env.discord.ident = &auth.Identity{Provider: "discord", ProviderUserID: "u2", Community: "artlab", DisplayName: "Emily"}

	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noRedirect.Get(env.srv.URL + "/api/auth/discord/start?return_to=/upcoming")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	providerURL := resp.Header.Get("Location")

	other := newClient(t)
	resp, err = other.Get(providerURL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Request.URL.Path != "/upcoming" {
		t.Fatalf("landed on %s", resp.Request.URL)
	}
	if code, _ := getMe(t, other, env.srv.URL); code != http.StatusOK {
		t.Fatalf("/me in other browser = %d", code)
	}

	// The state is single use.
	resp, err = newClient(t).Get(providerURL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Request.URL.Path != "/login" || resp.Request.URL.Query().Get("error") != "expired" {
		t.Fatalf("replayed state landed on %s", resp.Request.URL)
	}
}

func TestOAuthRejectedLogin(t *testing.T) {
	env := newTestEnv(t)
	env.discord.reject = auth.RejectNotInGuild
	c := newClient(t)
	resp, err := c.Get(env.srv.URL + "/api/auth/discord/start?return_to=/kiln")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	u := resp.Request.URL
	if u.Path != "/login" || u.Query().Get("error") != "not_in_guild" || u.Query().Get("return_to") != "/kiln" {
		t.Fatalf("landed on %s", u)
	}
	if code, _ := getMe(t, c, env.srv.URL); code != http.StatusUnauthorized {
		t.Fatalf("rejected user got a session")
	}
}

func TestLogoutRequiresCSRF(t *testing.T) {
	env := newTestEnv(t)
	env.discord.ident = &auth.Identity{Provider: "discord", ProviderUserID: "u3", Community: "artlab", DisplayName: "A"}
	c := newClient(t)
	resp, _ := c.Get(env.srv.URL + "/api/auth/discord/start")
	resp.Body.Close()

	resp, err := c.Post(env.srv.URL+"/api/auth/logout", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("logout without csrf = %d, want 403", resp.StatusCode)
	}

	u, _ := url.Parse(env.srv.URL)
	var csrf string
	for _, ck := range c.Jar.Cookies(u) {
		if ck.Name == csrfCookieName {
			csrf = ck.Value
		}
	}
	req, _ := http.NewRequest(http.MethodPost, env.srv.URL+"/api/auth/logout", strings.NewReader(""))
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout = %d", resp.StatusCode)
	}
	if code, _ := getMe(t, c, env.srv.URL); code != http.StatusUnauthorized {
		t.Fatalf("/me after logout = %d", code)
	}
}
