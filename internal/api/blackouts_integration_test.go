package api

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// Blackouts are admin-only, per kiln, and a hard block for everyone: nobody can
// propose or reschedule onto a blacked-out day. Existing firings are untouched.
func TestBlackouts(t *testing.T) {
	e := newFiringEnv(t)
	admin := e.loginWith("Ada", map[string]any{"admin": true})
	km := e.login("Kim", true)
	alice := e.login("Alice", false)

	blockedDay := dayOf(e.now.Add(72 * time.Hour))
	freeDay := dayOf(e.now.Add(96 * time.Hour))

	// Only admins manage blackouts.
	body := map[string]any{"kiln_id": 0, "start_date": blockedDay, "end_date": blockedDay, "reason": "elements"}
	km.mustDo("POST", "/api/admin/blackouts", body, http.StatusForbidden)
	alice.mustDo("POST", "/api/admin/blackouts", body, http.StatusForbidden)
	km.mustDo("GET", "/api/admin/blackouts", nil, http.StatusForbidden)

	opts := admin.mustDo("GET", "/api/options", nil, http.StatusOK)
	kilnID := int64(opts["kilns"].([]any)[0].(map[string]any)["id"].(float64))
	body["kiln_id"] = kilnID

	// Validation: end before start, and a missing kiln.
	admin.mustDo("POST", "/api/admin/blackouts", map[string]any{"kiln_id": kilnID,
		"start_date": freeDay, "end_date": blockedDay, "reason": ""}, http.StatusBadRequest)
	admin.mustDo("POST", "/api/admin/blackouts", map[string]any{"kiln_id": 0,
		"start_date": blockedDay, "end_date": blockedDay}, http.StatusBadRequest)

	created := admin.mustDo("POST", "/api/admin/blackouts", body, http.StatusCreated)
	blackoutID := int64(created["id"].(float64))

	// Nobody can book the blocked day, not even a kiln master.
	proposal := func(day string) map[string]any {
		return map[string]any{"firing_type": "glaze", "cone": "6", "scheduled_date": day,
			"description": "mugs", "share_pct": 20}
	}
	alice.mustDo("POST", "/api/firings", proposal(blockedDay), http.StatusConflict)
	km.mustDo("POST", "/api/firings", proposal(blockedDay), http.StatusConflict)

	// A free day still works, and rescheduling onto the blocked day is refused.
	d := alice.mustDo("POST", "/api/firings", proposal(freeDay), http.StatusCreated)
	base := fmt.Sprintf("/api/firings/%d", int64(jget(d, "firing", "id").(float64)))
	km.mustDo("POST", base+"/reschedule", map[string]any{"scheduled_date": blockedDay}, http.StatusConflict)

	// Options advertise the blackout so the propose form can warn.
	opts = alice.mustDo("GET", "/api/options", nil, http.StatusOK)
	if n := len(opts["blackouts"].([]any)); n != 1 {
		t.Fatalf("options blackouts = %v", opts["blackouts"])
	}

	// Removing it frees the day again.
	admin.mustDo("DELETE", fmt.Sprintf("/api/admin/blackouts/%d", blackoutID), nil, http.StatusNoContent)
	admin.mustDo("DELETE", fmt.Sprintf("/api/admin/blackouts/%d", blackoutID), nil, http.StatusNotFound)
	alice.mustDo("POST", "/api/firings", proposal(blockedDay), http.StatusCreated)
}
