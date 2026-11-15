package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"testing"

	"github.com/spencercornish/kilnlab/internal/auth"
	"github.com/spencercornish/kilnlab/internal/config"
	"github.com/spencercornish/kilnlab/internal/db"
	"github.com/spencercornish/kilnlab/internal/jobs"
)

func TestAdminSignsUserOut(t *testing.T) {
	e := newFiringEnv(t)
	boss := e.loginWith("Boss", map[string]any{"admin": true})
	mo := e.login("Mo", false)
	moPhone := e.login("Mo", false) // a second session for the same person
	moID := jget(mo.mustDo("GET", "/api/me", nil, http.StatusOK), "id")
	path := fmt.Sprintf("/api/admin/users/%v/sign-out", moID)

	mo.mustDo("POST", path, nil, http.StatusForbidden)
	boss.mustDo("POST", "/api/admin/users/999999/sign-out", nil, http.StatusNotFound)
	boss.mustDo("POST", path, nil, http.StatusNoContent)
	mo.mustDo("GET", "/api/me", nil, http.StatusUnauthorized)
	moPhone.mustDo("GET", "/api/me", nil, http.StatusUnauthorized)
	boss.mustDo("GET", "/api/me", nil, http.StatusOK)
}

func TestAuthStartIsRateLimited(t *testing.T) {
	env := newTestEnv(t)
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	status := func() int {
		resp, err := c.Get(env.srv.URL + "/api/auth/discord/start")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	for i := range authRateLimit {
		if got := status(); got != http.StatusFound {
			t.Fatalf("request %d: status %d", i+1, got)
		}
	}
	if got := status(); got != http.StatusTooManyRequests {
		t.Fatalf("over the limit: status %d, want 429", got)
	}
}

type fakeChecker map[string]any // provider user id → auth.Membership or error

func (f fakeChecker) CheckMember(_ context.Context, id string) (auth.Membership, error) {
	switch v := f[id].(type) {
	case auth.Membership:
		return v, nil
	case error:
		return auth.Membership{}, v
	}
	return auth.Membership{}, errors.New("unexpected lookup of " + id)
}

func TestJobsCleanUpAndRecheckMembers(t *testing.T) {
	e, pool := newFiringEnvPool(t)
	ctx := context.Background()
	kim := e.login("Kim", true)     // still a member, lost the role
	gone := e.login("Gone", true)   // left the server
	flaky := e.login("Flaky", true) // Discord can't answer
	mo := e.login("Mo", false)      // promoted to kiln master

	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (id, user_id, expires_at) SELECT 'old', id, now() - interval '1 hour' FROM users LIMIT 1;
		INSERT INTO oauth_states (state, provider, return_to, browser_hash, expires_at) VALUES
			('old', 'discord', '/', 'h', now() - interval '1 minute'), ('live', 'discord', '/', 'h', now() + interval '5 minutes')`); err != nil {
		t.Fatal(err)
	}
	runner := jobs.NewRunner(pool, NotifyConfig(&config.Config{AppBaseURL: "http://k"}), nil, slog.New(slog.DiscardHandler))
	if _, err := runner.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	var sessions, states int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM sessions), (SELECT count(*) FROM oauth_states)`).Scan(&sessions, &states); err != nil {
		t.Fatal(err)
	}
	if sessions != 4 || states != 1 {
		t.Fatalf("after cleanup: %d sessions, %d oauth states; want 4 and 1", sessions, states)
	}

	runner.Checkers[db.AuthProviderDiscord] = fakeChecker{
		"dev-kim":   auth.Membership{Member: true},
		"dev-gone":  auth.Membership{},
		"dev-flaky": errors.New("discord is down"),
		"dev-mo":    auth.Membership{Member: true, KilnMaster: true},
	}
	if err := runner.RecheckMembers(ctx); err == nil {
		t.Fatal("want the failed lookup reported")
	}
	roles := func(c *apiClient) any {
		return jget(c.mustDo("GET", "/api/me", nil, http.StatusOK), "roles", "kiln_master")
	}
	if roles(kim) != false || roles(mo) != true {
		t.Fatalf("kiln master: kim=%v mo=%v, want false and true", roles(kim), roles(mo))
	}
	gone.mustDo("GET", "/api/me", nil, http.StatusUnauthorized)
	if roles(flaky) != true { // an outage must not take access away
		t.Fatal("a failed lookup changed access")
	}
	var goneKM bool
	if err := pool.QueryRow(ctx, `SELECT is_kiln_master FROM accounts WHERE provider_user_id = 'dev-gone'`).Scan(&goneKM); err != nil || goneKM {
		t.Fatalf("gone still a kiln master (%v, %v)", goneKM, err)
	}
}
