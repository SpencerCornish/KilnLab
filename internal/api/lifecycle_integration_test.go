package api

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func parseTime(t *testing.T, v any) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, v.(string))
	if err != nil {
		t.Fatalf("parse %v: %v", v, err)
	}
	return ts
}

func TestLifecycleAndKilnFlow(t *testing.T) {
	e := newFiringEnv(t)
	km := e.login("Kim", true)
	sam := e.login("Sam", false)
	mo := e.login("Mo", false)

	body := firingBody("glaze", "6", dayOf(e.now.Add(2*time.Hour)), "mugs", 50)
	custom := body["custom"].(map[string]any)
	custom["firing_minutes"] = 600
	custom["cooldown_minutes"] = 0
	d := km.mustDo("POST", "/api/firings", body, http.StatusCreated)
	id := int64(jget(d, "firing", "id").(float64))
	base := fmt.Sprintf("/api/firings/%d", id)
	status := func(c *apiClient, body map[string]any, want int) map[string]any {
		t.Helper()
		return c.mustDo("POST", base+"/status", body, want)
	}

	// An unclaimed proposed firing is visible at the kiln so a KM can take it.
	if n := len(km.mustDo("GET", "/api/kiln", nil, http.StatusOK)["firings"].([]any)); n != 1 {
		t.Fatalf("kiln view before claim: %d firings", n)
	}
	status(km, map[string]any{"to": "firing"}, http.StatusConflict) // claim first
	km.mustDo("POST", base+"/claim", nil, http.StatusOK)

	kv := mo.mustDo("GET", "/api/kiln?k=1", nil, http.StatusOK)
	if len(kv["firings"].([]any)) != 1 || jget(kv, "kiln", "id") != 1.0 {
		t.Fatalf("kiln view = %v", kv)
	}
	mo.mustDo("GET", "/api/kiln?k=999999", nil, http.StatusNotFound)

	// Members don't get to start the kiln, but can unload once it has started.
	d = sam.mustDo("GET", base, nil, http.StatusOK)
	if steps := jget(d, "can", "next_steps").([]any); len(steps) != 0 {
		t.Fatalf("sam's next steps = %v", steps)
	}
	if jget(d, "can", "blocked_reason") != "Only kiln masters can start the kiln." {
		t.Fatalf("blocked reason = %v", jget(d, "can"))
	}

	// Only kiln masters start the kiln; back-dating is checked.
	status(sam, map[string]any{"to": "firing"}, http.StatusForbidden)
	status(km, map[string]any{"to": "firing", "occurred_at": e.now.Add(time.Hour)}, http.StatusBadRequest)
	startedAt := e.now.Add(-30 * time.Minute)
	d = status(km, map[string]any{"to": "firing", "occurred_at": startedAt, "source": "kiln_qr"}, http.StatusOK)
	f := d["firing"].(map[string]any)
	if f["status"] != "firing" || !parseTime(t, f["started_at"]).Equal(startedAt) ||
		!parseTime(t, f["firing_ends_at"]).Equal(startedAt.Add(10*time.Hour)) ||
		!parseTime(t, f["unloadable_at"]).Equal(startedAt.Add(10*time.Hour)) {
		t.Fatalf("started firing = %v", f)
	}

	// Correcting the start moves both estimates; members can't correct times.
	e.advance(9 * time.Hour)
	km.mustDo("PUT", base+"/timestamps", map[string]any{"field": "unloaded_at", "at": e.now}, http.StatusConflict)
	sam.mustDo("PUT", base+"/timestamps", map[string]any{"field": "started_at", "at": startedAt}, http.StatusForbidden)
	newStart := startedAt.Add(-15 * time.Minute)
	d = km.mustDo("PUT", base+"/timestamps", map[string]any{"field": "started_at", "at": newStart}, http.StatusOK)
	if got := parseTime(t, jget(d, "firing", "unloadable_at")); !got.Equal(newStart.Add(10 * time.Hour)) {
		t.Fatalf("unloadable after correction = %v", got)
	}

	// Anyone can unload, even before the estimate.
	d = status(sam, map[string]any{"to": "completed", "source": "kiln_qr"}, http.StatusOK)
	if jget(d, "firing", "unloaded_at") == nil || len(jget(d, "can", "next_steps").([]any)) != 0 {
		t.Fatalf("completed = %v", d)
	}
	if n := len(km.mustDo("GET", "/api/kiln", nil, http.StatusOK)["firings"].([]any)); n != 0 {
		t.Fatalf("completed firing still at the kiln (%d)", n)
	}

	kilnQR := 0
	for _, ev := range d["events"].([]any) {
		if ev.(map[string]any)["source"] == "kiln_qr" {
			kilnQR++
		}
	}
	if kilnQR != 2 {
		t.Errorf("kiln_qr events = %d, want 2", kilnQR)
	}
}

func TestKilnMasterSkipsToStart(t *testing.T) {
	e := newFiringEnv(t)
	km := e.login("Kim", true)
	d := km.mustDo("POST", "/api/firings", firingBody("bisque", "04", dayOf(e.now.Add(time.Hour)), "bowls", 80), http.StatusCreated)
	base := fmt.Sprintf("/api/firings/%d", int64(jget(d, "firing", "id").(float64)))
	d = km.mustDo("POST", base+"/claim", nil, http.StatusOK)
	if steps := jget(d, "can", "next_steps").([]any); len(steps) != 1 || steps[0] != "firing" {
		t.Fatalf("KM steps at scheduled = %v", steps)
	}
	km.mustDo("POST", base+"/status", map[string]any{"to": "firing"}, http.StatusOK)
	km.mustDo("POST", base+"/status", map[string]any{"to": "scheduled"}, http.StatusConflict) // no going back
}

func TestManagePeople(t *testing.T) {
	e := newFiringEnv(t)
	km := e.login("Kim", true)
	boss := e.loginWith("Boss", map[string]any{"admin": true})
	mo := e.login("Mo", false)
	moID := jget(mo.mustDo("GET", "/api/me", nil, http.StatusOK), "id")
	bossID := jget(boss.mustDo("GET", "/api/me", nil, http.StatusOK), "id")
	user := func(id any) string { return fmt.Sprintf("/api/admin/users/%v", id) }

	// Only admins see the people page; kiln masters don't manage people.
	mo.mustDo("GET", "/api/admin/users", nil, http.StatusForbidden)
	km.mustDo("GET", "/api/admin/users", nil, http.StatusForbidden)
	users := boss.mustDo("GET", "/api/admin/users", nil, http.StatusOK)["items"].([]any)
	if len(users) != 3 {
		t.Fatalf("users = %v", users)
	}

	km.mustDo("PATCH", user(moID), map[string]any{"admin": true}, http.StatusForbidden)
	boss.mustDo("PATCH", user(moID), map[string]any{"admin": true}, http.StatusNoContent)
	if me := mo.mustDo("GET", "/api/me", nil, http.StatusOK); jget(me, "roles", "admin") != true {
		t.Fatalf("mo roles = %v", me["roles"])
	}
	boss.mustDo("PATCH", user(bossID), map[string]any{"admin": false}, http.StatusConflict)
	boss.mustDo("PATCH", user(999999), map[string]any{"admin": true}, http.StatusNotFound)
}
