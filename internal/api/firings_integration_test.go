package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/spencercornish/kilnlab/internal/config"
)

// firingEnv is a server with dev login and a controllable clock.
type firingEnv struct {
	t   *testing.T
	srv *httptest.Server
	mu  sync.Mutex
	now time.Time
}

func newFiringEnv(t *testing.T) *firingEnv {
	env, _ := newFiringEnvPool(t)
	return env
}

func newFiringEnvPool(t *testing.T) (*firingEnv, *pgxpool.Pool) {
	pool := openTestPool(t)
	env := &firingEnv{t: t, now: time.Now().UTC().Truncate(time.Second)} // sessions expire against the DB clock
	cfg := &config.Config{AppBaseURL: "http://localhost", DevLogin: true}
	s := New(cfg, pool, nil, nil, slog.New(slog.DiscardHandler))
	s.now = func() time.Time { env.mu.Lock(); defer env.mu.Unlock(); return env.now }
	env.srv = httptest.NewServer(s.Handler())
	t.Cleanup(env.srv.Close)
	return env, pool
}

func (e *firingEnv) advance(d time.Duration) { e.mu.Lock(); e.now = e.now.Add(d); e.mu.Unlock() }

type apiClient struct {
	e *firingEnv
	c *http.Client
}

// login signs in through the dev login endpoint.
func (e *firingEnv) login(name string, km, selfLoader bool) *apiClient {
	return e.loginWith(name, map[string]any{"kiln_master": km, "self_loader": selfLoader})
}

func (e *firingEnv) loginWith(name string, flags map[string]any) *apiClient {
	c := &apiClient{e: e, c: newClient(e.t)}
	body := map[string]any{"name": name, "community": "artlab"}
	for k, v := range flags {
		body[k] = v
	}
	c.mustDo(http.MethodPost, "/api/auth/dev", body, http.StatusNoContent)
	return c
}

func (c *apiClient) do(method, path string, body any) (int, map[string]any) {
	c.e.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	if method != http.MethodGet && c.csrf() == "" {
		resp, err := c.c.Get(c.e.srv.URL + "/api/health")
		if err != nil {
			c.e.t.Fatal(err)
		}
		resp.Body.Close()
	}
	req, _ := http.NewRequest(method, c.e.srv.URL+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", c.csrf())
	resp, err := c.c.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if resp.StatusCode != http.StatusNoContent {
		var raw any
		_ = json.NewDecoder(resp.Body).Decode(&raw)
		switch v := raw.(type) {
		case map[string]any:
			out = v
		case []any:
			out = map[string]any{"items": v}
		}
	}
	return resp.StatusCode, out
}

func (c *apiClient) mustDo(method, path string, body any, want int) map[string]any {
	c.e.t.Helper()
	code, out := c.do(method, path, body)
	if code != want {
		c.e.t.Fatalf("%s %s = %d (%v), want %d", method, path, code, out, want)
	}
	return out
}

func (c *apiClient) csrf() string {
	u, _ := url.Parse(c.e.srv.URL)
	for _, ck := range c.c.Jar.Cookies(u) {
		if ck.Name == csrfCookieName {
			return ck.Value
		}
	}
	return ""
}

func jget(m map[string]any, keys ...string) any {
	var v any = m
	for _, k := range keys {
		v = v.(map[string]any)[k]
	}
	return v
}

func TestFiringsAndRequestsFlow(t *testing.T) {
	e := newFiringEnv(t)
	alice := e.login("Alice", false, false) // proposer
	bob := e.login("Bob", false, false)     // requester
	km := e.login("Kim", true, false)       // kiln master

	start := e.now.Add(72 * time.Hour)
	proposal := map[string]any{
		"firing_type": "glaze", "cone": "6", "scheduled_start": start,
		"description": "4 mugs", "share_pct": 30,
	}

	// Minimum notice: members need 48h, kiln masters can bypass.
	soon := map[string]any{"firing_type": "glaze", "cone": "6", "scheduled_start": e.now.Add(24 * time.Hour),
		"description": "x", "share_pct": 10}
	alice.mustDo("POST", "/api/firings", soon, http.StatusBadRequest)
	km.mustDo("POST", "/api/firings", soon, http.StatusCreated)
	alice.mustDo("POST", "/api/firings", map[string]any{"firing_type": "raku", "cone": "6",
		"scheduled_start": start, "description": "x", "share_pct": 10}, http.StatusBadRequest)

	d := alice.mustDo("POST", "/api/firings", proposal, http.StatusCreated)
	id := int64(jget(d, "firing", "id").(float64))
	base := fmt.Sprintf("/api/firings/%d", id)
	if jget(d, "firing", "status") != "proposed" || jget(d, "firing", "fullness", "display_pct") != 30.0 {
		t.Fatalf("created firing = %v", d["firing"])
	}
	if jget(d, "can", "request") != false || jget(d, "can", "confirm") != false {
		t.Fatalf("proposer perms = %v", d["can"])
	}
	alice.mustDo("POST", base+"/requests", map[string]any{"description": "x", "share_pct": 5, "cone": "6"}, http.StatusConflict)

	// Bob requests with the wrong cone: allowed, with a warning.
	d = bob.mustDo("POST", base+"/requests", map[string]any{
		"description": "2 bowls", "share_pct": 80, "cone": "5", "glazed": true,
	}, http.StatusOK)
	warnings := d["warnings"].([]any)
	if len(warnings) != 2 { // cone mismatch + may not fit (30 + 80)
		t.Fatalf("warnings = %v", warnings)
	}
	my := d["my_request"].(map[string]any)
	rid := int64(my["id"].(float64))
	if my["compatibility_warning"] == nil {
		t.Fatal("compat warning should be visible to the requester")
	}
	bob.mustDo("POST", base+"/requests", map[string]any{"description": "again", "share_pct": 5, "cone": "6"}, http.StatusConflict)

	// Bob edits down to fit.
	d = bob.mustDo("PUT", fmt.Sprintf("%s/requests/%d", base, rid), map[string]any{
		"description": "2 bowls", "share_pct": 40, "cone": "6", "glazed": true,
	}, http.StatusOK)
	if w := d["warnings"].([]any); len(w) != 0 {
		t.Fatalf("warnings after fix = %v", w)
	}

	// Only kiln masters decide (open question 1 default).
	decide := fmt.Sprintf("%s/requests/%d/decide", base, rid)
	alice.mustDo("POST", decide, map[string]any{"decision": "approve"}, http.StatusForbidden)
	bob.mustDo("POST", decide, map[string]any{"decision": "approve"}, http.StatusForbidden)
	d = km.mustDo("POST", decide, map[string]any{"decision": "approve", "note": "fits"}, http.StatusOK)
	if jget(d, "firing", "fullness", "display_pct") != 70.0 {
		t.Fatalf("fullness after approve = %v", jget(d, "firing", "fullness"))
	}

	// Other members see the live request but not the decision note.
	d = alice.mustDo("GET", base, nil, http.StatusOK)
	reqs := d["requests"].([]any)
	if len(reqs) != 1 || reqs[0].(map[string]any)["decision_note"] != nil {
		t.Fatalf("alice sees requests = %v", reqs)
	}

	// Override fullness: kiln master only.
	alice.mustDo("PUT", base+"/fullness-override", map[string]any{"override_pct": 90}, http.StatusForbidden)
	d = km.mustDo("PUT", base+"/fullness-override", map[string]any{"override_pct": 90}, http.StatusOK)
	if f := jget(d, "firing", "fullness").(map[string]any); f["display_pct"] != 90.0 || f["overridden"] != true {
		t.Fatalf("override = %v", f)
	}
	km.mustDo("PUT", base+"/fullness-override", map[string]any{"override_pct": nil}, http.StatusOK)

	// Confirm, hold, release back to confirmed.
	alice.mustDo("POST", base+"/confirm", nil, http.StatusForbidden)
	km.mustDo("POST", base+"/confirm", nil, http.StatusOK)
	km.mustDo("POST", base+"/confirm", nil, http.StatusConflict)
	km.mustDo("POST", base+"/hold", map[string]any{"reason": ""}, http.StatusBadRequest)
	d = km.mustDo("POST", base+"/hold", map[string]any{"reason": "not enough work"}, http.StatusOK)
	if jget(d, "firing", "hold_reason") != "not enough work" || jget(d, "can", "release") != true {
		t.Fatalf("held = %v / %v", d["firing"], d["can"])
	}
	newStart := start.Add(24 * time.Hour)
	d = km.mustDo("POST", base+"/release", map[string]any{"scheduled_start": newStart}, http.StatusOK)
	if jget(d, "firing", "status") != "confirmed" || jget(d, "firing", "hold_reason") != nil {
		t.Fatalf("released = %v", d["firing"])
	}
	if got, _ := time.Parse(time.RFC3339, jget(d, "firing", "scheduled_start").(string)); !got.Equal(newStart) {
		t.Fatalf("release reschedule: start = %v", got)
	}
	wantClose := newStart.Add(-12 * time.Hour)
	if got, _ := time.Parse(time.RFC3339, jget(d, "firing", "requests_close_at").(string)); !got.Equal(wantClose) {
		t.Fatalf("close_at = %v, want %v", got, wantClose)
	}

	// Close/reopen requests.
	carol := e.login("Carol", false, false)
	km.mustDo("PUT", base+"/requests-window", map[string]any{"closed": true}, http.StatusOK)
	carol.mustDo("POST", base+"/requests", map[string]any{"description": "x", "share_pct": 5, "cone": "6"}, http.StatusConflict)
	bob.mustDo("POST", fmt.Sprintf("%s/requests/%d/withdraw", base, rid), nil, http.StatusConflict)
	km.mustDo("PUT", base+"/requests-window", map[string]any{"closed": false}, http.StatusOK)
	d = carol.mustDo("POST", base+"/requests", map[string]any{"description": "a vase", "share_pct": 5, "cone": "6"}, http.StatusOK)
	carolRid := int64(jget(d, "my_request", "id").(float64))

	// The automatic cutoff: 12h before the start.
	e.advance(newStart.Sub(e.now) - 12*time.Hour)
	d = carol.mustDo("GET", base, nil, http.StatusOK)
	if jget(d, "firing", "requests_open") != false || jget(d, "my_request", "can", "withdraw") != false {
		t.Fatalf("after cutoff: %v", d)
	}
	carol.mustDo("POST", fmt.Sprintf("%s/requests/%d/withdraw", base, carolRid), nil, http.StatusConflict)
	dave := e.login("Dave", false, false)
	dave.mustDo("POST", base+"/requests", map[string]any{"description": "x", "share_pct": 5, "cone": "6"}, http.StatusConflict)

	// Reopening after the cutoff extends the window up to the start.
	km.mustDo("PUT", base+"/requests-window", map[string]any{"closed": false}, http.StatusOK)
	carol.mustDo("POST", fmt.Sprintf("%s/requests/%d/withdraw", base, carolRid), nil, http.StatusOK)

	// Reschedule (kiln master only), then cancel with a reason.
	later := newStart.Add(48 * time.Hour)
	alice.mustDo("POST", base+"/reschedule", map[string]any{"scheduled_start": later}, http.StatusForbidden)
	km.mustDo("POST", base+"/reschedule", map[string]any{"scheduled_start": e.now.Add(-time.Hour)}, http.StatusBadRequest)
	km.mustDo("POST", base+"/reschedule", map[string]any{"scheduled_start": later}, http.StatusOK)
	km.mustDo("POST", base+"/cancel", map[string]any{"reason": ""}, http.StatusBadRequest)
	d = km.mustDo("POST", base+"/cancel", map[string]any{"reason": "kiln broken"}, http.StatusOK)
	if jget(d, "firing", "status") != "cancelled" || jget(d, "firing", "cancel_reason") != "kiln broken" {
		t.Fatalf("cancelled = %v", d["firing"])
	}
	km.mustDo("POST", base+"/reschedule", map[string]any{"scheduled_start": later}, http.StatusConflict)
	km.mustDo("POST", decide, map[string]any{"decision": "decline"}, http.StatusConflict)

	// Every change wrote a timeline event.
	types := map[string]int{}
	for _, ev := range d["events"].([]any) {
		types[ev.(map[string]any)["type"].(string)]++
	}
	for typ, n := range map[string]int{
		"created": 1, "request_submitted": 2, "request_updated": 1, "request_approved": 1,
		"request_withdrawn": 1, "fullness_override": 2, "status_change": 4, "rescheduled": 2,
		"requests_closed": 1, "requests_reopened": 2,
	} {
		if types[typ] != n {
			t.Errorf("events[%s] = %d, want %d (all: %v)", typ, types[typ], n, types)
		}
	}

	// The list endpoint flags firings I'm in.
	list := bob.mustDo("GET", "/api/firings", nil, http.StatusOK)["items"].([]any)
	mine := 0
	for _, f := range list {
		if f.(map[string]any)["is_mine"] == true {
			mine++
		}
	}
	if len(list) != 2 || mine != 1 {
		t.Fatalf("bob's list: %d firings, %d mine", len(list), mine)
	}
}

func TestLoaderAssignment(t *testing.T) {
	e := newFiringEnv(t)
	km := e.login("Kim", true, false)
	sam := e.login("Sam", false, true) // self-loader
	mo := e.login("Mo", false, false)

	d := km.mustDo("POST", "/api/firings", map[string]any{"firing_type": "bisque", "cone": "04",
		"scheduled_start": e.now.Add(72 * time.Hour), "description": "greenware", "share_pct": 50}, http.StatusCreated)
	base := fmt.Sprintf("/api/firings/%d", int64(jget(d, "firing", "id").(float64)))

	cands := km.mustDo("GET", "/api/loaders", nil, http.StatusOK)["items"].([]any)
	if len(cands) != 2 { // Kim and Sam
		t.Fatalf("loader candidates = %v", cands)
	}
	mo.mustDo("GET", "/api/loaders", nil, http.StatusForbidden)

	samID := jget(sam.mustDo("GET", "/api/me", nil, http.StatusOK), "id")
	moID := jget(mo.mustDo("GET", "/api/me", nil, http.StatusOK), "id")
	km.mustDo("PUT", base+"/loader", map[string]any{"user_id": moID}, http.StatusBadRequest)
	sam.mustDo("PUT", base+"/loader", map[string]any{"user_id": samID}, http.StatusForbidden)
	d = km.mustDo("PUT", base+"/loader", map[string]any{"user_id": samID}, http.StatusOK)
	if jget(d, "loader", "name") != "Sam" {
		t.Fatalf("loader = %v", d["loader"])
	}
}

func TestMultipleKilns(t *testing.T) {
	e, pool := newFiringEnvPool(t)
	ctx := t.Context()
	var second, retired int64
	if err := pool.QueryRow(ctx, `INSERT INTO kilns (name, shelf_count) VALUES ('Test kiln', 4) RETURNING id`).Scan(&second); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO kilns (name, active) VALUES ('Old kiln', false) RETURNING id`).Scan(&retired); err != nil {
		t.Fatal(err)
	}
	// (openTestPool removes extra kilns before each test.)

	alice := e.login("Alice", false, false)
	kilns := alice.mustDo("GET", "/api/options", nil, http.StatusOK)["kilns"].([]any)
	if len(kilns) != 3 || kilns[2].(map[string]any)["active"] != false {
		t.Fatalf("options kilns = %v", kilns)
	}

	body := func(kilnID int64) map[string]any {
		return map[string]any{"kiln_id": kilnID, "firing_type": "glaze", "cone": "6",
			"scheduled_start": e.now.Add(72 * time.Hour), "description": "mugs", "share_pct": 20}
	}
	// With two active kilns the kiln must be chosen, and must be active.
	alice.mustDo("POST", "/api/firings", body(0), http.StatusBadRequest)
	alice.mustDo("POST", "/api/firings", body(retired), http.StatusBadRequest)
	alice.mustDo("POST", "/api/firings", body(999999), http.StatusBadRequest)
	d := alice.mustDo("POST", "/api/firings", body(second), http.StatusCreated)
	if jget(d, "firing", "kiln_id") != float64(second) {
		t.Fatalf("kiln_id = %v, want %d", jget(d, "firing", "kiln_id"), second)
	}
}
