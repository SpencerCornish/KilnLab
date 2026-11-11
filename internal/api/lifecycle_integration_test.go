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
	km := e.login("Kim", true, false)
	sam := e.login("Sam", false, true) // self-loader
	mo := e.login("Mo", false, false)

	d := km.mustDo("POST", "/api/firings", map[string]any{"firing_type": "glaze", "cone": "6",
		"scheduled_start": e.now.Add(2 * time.Hour), "description": "mugs", "share_pct": 50}, http.StatusCreated)
	id := int64(jget(d, "firing", "id").(float64))
	base := fmt.Sprintf("/api/firings/%d", id)
	status := func(c *apiClient, body map[string]any, want int) map[string]any {
		t.Helper()
		return c.mustDo("POST", base+"/status", body, want)
	}

	// Proposed firings aren't at the kiln yet, and can't be loaded before confirming.
	if n := len(km.mustDo("GET", "/api/kiln", nil, http.StatusOK)["firings"].([]any)); n != 0 {
		t.Fatalf("kiln view before confirm: %d firings", n)
	}
	status(km, map[string]any{"to": "loading"}, http.StatusConflict)
	km.mustDo("POST", base+"/confirm", nil, http.StatusOK)

	kv := mo.mustDo("GET", "/api/kiln?k=1", nil, http.StatusOK)
	if len(kv["firings"].([]any)) != 1 || jget(kv, "kiln", "id") != 1.0 {
		t.Fatalf("kiln view = %v", kv)
	}
	mo.mustDo("GET", "/api/kiln?k=999999", nil, http.StatusNotFound)

	// Unassigned members/self-loaders can't touch it; the assigned loader can.
	status(sam, map[string]any{"to": "loading"}, http.StatusForbidden)
	samID := jget(sam.mustDo("GET", "/api/me", nil, http.StatusOK), "id")
	km.mustDo("PUT", base+"/loader", map[string]any{"user_id": samID}, http.StatusOK)
	d = sam.mustDo("GET", base, nil, http.StatusOK)
	if steps := jget(d, "can", "next_steps").([]any); len(steps) != 1 || steps[0] != "loading" {
		t.Fatalf("sam's next steps = %v", steps)
	}
	status(mo, map[string]any{"to": "loading"}, http.StatusForbidden)
	status(sam, map[string]any{"to": "loading", "source": "kiln_qr"}, http.StatusOK)
	d = status(sam, map[string]any{"to": "loaded", "note": "top shelf has the tall vase"}, http.StatusOK)
	if jget(d, "can", "blocked_reason") != "Only kiln masters can start the kiln." {
		t.Fatalf("blocked reason = %v", jget(d, "can"))
	}

	// Only kiln masters start the kiln, and starting needs a program + duration.
	status(sam, map[string]any{"to": "firing", "program": "Glaze", "estimated_duration_minutes": 600}, http.StatusForbidden)
	status(km, map[string]any{"to": "firing"}, http.StatusBadRequest)
	status(km, map[string]any{"to": "firing", "program": "Glaze"}, http.StatusBadRequest)
	// Back-dating: not into the future, not more than 7 days back.
	status(km, map[string]any{"to": "firing", "program": "Glaze", "estimated_duration_minutes": 600,
		"occurred_at": e.now.Add(time.Hour)}, http.StatusBadRequest)
	startedAt := e.now.Add(-30 * time.Minute)
	d = status(km, map[string]any{"to": "firing", "program": "Slow Glaze", "estimated_duration_minutes": 600,
		"occurred_at": startedAt, "source": "kiln_qr"}, http.StatusOK)
	f := d["firing"].(map[string]any)
	if f["status"] != "firing" || !parseTime(t, f["started_at"]).Equal(startedAt) ||
		!parseTime(t, f["estimated_unload_at"]).Equal(startedAt.Add(10*time.Hour)) || f["program"] != "Slow Glaze" {
		t.Fatalf("started firing = %v", f)
	}

	// Self-loaders can't back-date; a time near "now" is accepted as now.
	e.advance(9 * time.Hour)
	status(sam, map[string]any{"to": "unloaded", "occurred_at": e.now.Add(-time.Hour)}, http.StatusForbidden)
	coolAt := e.now.Add(-2 * time.Hour)
	status(km, map[string]any{"to": "cooling", "occurred_at": startedAt.Add(-time.Minute)}, http.StatusBadRequest) // before start
	status(km, map[string]any{"to": "cooling", "occurred_at": coolAt}, http.StatusOK)

	// Correct the start time: est. unload follows; order is enforced.
	km.mustDo("PUT", base+"/timestamps", map[string]any{"field": "unloaded_at", "at": e.now}, http.StatusConflict)
	km.mustDo("PUT", base+"/timestamps", map[string]any{"field": "started_at", "at": coolAt.Add(time.Minute)}, http.StatusBadRequest)
	sam.mustDo("PUT", base+"/timestamps", map[string]any{"field": "started_at", "at": startedAt}, http.StatusForbidden)
	newStart := startedAt.Add(-15 * time.Minute)
	d = km.mustDo("PUT", base+"/timestamps", map[string]any{"field": "started_at", "at": newStart}, http.StatusOK)
	if got := parseTime(t, jget(d, "firing", "estimated_unload_at")); !got.Equal(newStart.Add(10 * time.Hour)) {
		t.Fatalf("est unload after correction = %v", got)
	}

	status(sam, map[string]any{"to": "unloaded", "source": "kiln_qr"}, http.StatusOK)
	status(sam, map[string]any{"to": "ready_for_pickup"}, http.StatusForbidden)
	status(km, map[string]any{"to": "ready_for_pickup"}, http.StatusOK)
	d = status(km, map[string]any{"to": "completed"}, http.StatusOK)
	if jget(d, "firing", "completed_at") == nil || len(jget(d, "can", "next_steps").([]any)) != 0 {
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
	if kilnQR != 3 {
		t.Errorf("kiln_qr events = %d, want 3", kilnQR)
	}
}

func TestKilnMasterSkipsToStart(t *testing.T) {
	e := newFiringEnv(t)
	km := e.login("Kim", true, false)
	d := km.mustDo("POST", "/api/firings", map[string]any{"firing_type": "bisque", "cone": "04",
		"scheduled_start": e.now.Add(time.Hour), "description": "bowls", "share_pct": 80}, http.StatusCreated)
	base := fmt.Sprintf("/api/firings/%d", int64(jget(d, "firing", "id").(float64)))
	d = km.mustDo("POST", base+"/confirm", nil, http.StatusOK)
	if steps := jget(d, "can", "next_steps").([]any); len(steps) != 2 || steps[1] != "firing" {
		t.Fatalf("KM steps at confirmed = %v", steps)
	}
	km.mustDo("POST", base+"/status", map[string]any{"to": "firing", "program": "Bisque", "estimated_duration_minutes": 480}, http.StatusOK)
	km.mustDo("POST", base+"/status", map[string]any{"to": "loading"}, http.StatusConflict) // no going back
}

func TestManagePeople(t *testing.T) {
	e := newFiringEnv(t)
	km := e.login("Kim", true, false)
	boss := e.loginWith("Boss", map[string]any{"admin": true})
	mo := e.login("Mo", false, false)
	moID := jget(mo.mustDo("GET", "/api/me", nil, http.StatusOK), "id")
	bossID := jget(boss.mustDo("GET", "/api/me", nil, http.StatusOK), "id")
	user := func(id any) string { return fmt.Sprintf("/api/admin/users/%v", id) }

	mo.mustDo("GET", "/api/admin/users", nil, http.StatusForbidden)
	users := km.mustDo("GET", "/api/admin/users", nil, http.StatusOK)["items"].([]any)
	if len(users) != 3 {
		t.Fatalf("users = %v", users)
	}

	mo.mustDo("PATCH", user(moID), map[string]any{"self_loader": true}, http.StatusForbidden)
	km.mustDo("PATCH", user(moID), map[string]any{"self_loader": true}, http.StatusNoContent)
	if me := mo.mustDo("GET", "/api/me", nil, http.StatusOK); jget(me, "roles", "self_loader") != true {
		t.Fatalf("mo roles = %v", me["roles"])
	}
	km.mustDo("PATCH", user(moID), map[string]any{"admin": true}, http.StatusForbidden)
	boss.mustDo("PATCH", user(moID), map[string]any{"admin": true}, http.StatusNoContent)
	boss.mustDo("PATCH", user(bossID), map[string]any{"admin": false}, http.StatusConflict)
	boss.mustDo("PATCH", user(999999), map[string]any{"self_loader": true}, http.StatusNotFound)
}
