package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPublicUpcomingAndCalendar(t *testing.T) {
	e := newFiringEnv(t)
	alice := e.login("Alice", false)
	bob := e.login("Bob", false)
	km := e.login("Kim", true)

	mk := func(c *apiClient, in time.Duration, share int) string {
		body := firingBody("glaze", "6", dayOf(e.now.Add(in)), "secret notes", share)
		body["notes"] = "my private note"
		d := c.mustDo("POST", "/api/firings", body, http.StatusCreated)
		return fmt.Sprintf("/api/firings/%d", int64(jget(d, "firing", "id").(float64)))
	}
	soon := mk(alice, 72*time.Hour, 40)
	far := mk(alice, 60*24*time.Hour, 10) // beyond the 30-day window
	cancelled := mk(km, 96*time.Hour, 10)
	km.mustDo("POST", cancelled+"/cancel", map[string]any{"reason": "nope"}, http.StatusOK)
	d := bob.mustDo("POST", soon+"/requests", map[string]any{"description": "bowls", "share_pct": 35, "cone": "6"}, http.StatusOK)
	km.mustDo("POST", fmt.Sprintf("%s/requests/%d/decide", soon, int64(jget(d, "my_request", "id").(float64))),
		map[string]any{"decision": "approve"}, http.StatusOK)
	_ = far

	// No login needed; no names or notes leak.
	anon := &apiClient{e: e, c: newClient(t)}
	resp, err := anon.c.Get(e.srv.URL + "/api/public/upcoming")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upcoming = %d %s", resp.StatusCode, body)
	}
	for _, leak := range []string{"Alice", "Bob", "secret", "private"} {
		if strings.Contains(string(body), leak) {
			t.Errorf("public page leaks %q: %s", leak, body)
		}
	}
	code, up := anon.do("GET", "/api/public/upcoming", nil)
	firings := up["firings"].([]any)
	if code != 200 || len(firings) != 1 {
		t.Fatalf("want only the soon firing, got %d: %v", len(firings), firings)
	}
	f := firings[0].(map[string]any)
	if f["fullness_pct"] != 75.0 || f["room_left_pct"] != 25.0 || f["participant_count"] != 2.0 || f["requests_open"] != true {
		t.Fatalf("public firing = %v", f)
	}

	// Calendar: the non-cancelled firings in the window, as text/calendar.
	resp, err = anon.c.Get(e.srv.URL + "/calendar.ics")
	if err != nil {
		t.Fatal(err)
	}
	ics, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/calendar") {
		t.Fatalf("content type %q", ct)
	}
	if n := strings.Count(string(ics), "BEGIN:VEVENT"); n != 1 {
		t.Fatalf("events = %d\n%s", n, ics)
	}
	if !strings.Contains(string(ics), "SUMMARY:Cone 6 glaze (75% full)") || !strings.Contains(string(ics), "STATUS:TENTATIVE") {
		t.Fatalf("ics =\n%s", ics)
	}
}

func TestAdminSettings(t *testing.T) {
	e := newFiringEnv(t)
	km := e.login("Kim", true)
	admin := e.loginWith("Ada", map[string]any{"admin": true})

	km.mustDo("GET", "/api/admin/settings", nil, http.StatusForbidden)
	got := admin.mustDo("GET", "/api/admin/settings", nil, http.StatusOK)
	st := got["settings"].(map[string]any)
	if st["timezone"] != "America/Denver" || len(got["kilns"].([]any)) != 1 || st["day_reminder_hour"] != 8.0 {
		t.Fatalf("settings = %v", got)
	}

	st["min_notice_hours"] = 24
	st["day_reminder_hour"] = 6
	st["escalation_grace_minutes"] = 90
	st["cones"] = []string{" 6 ", "10", ""}
	got = admin.mustDo("PUT", "/api/admin/settings", st, http.StatusOK)
	st = got["settings"].(map[string]any)
	if st["min_notice_hours"] != 24.0 || st["day_reminder_hour"] != 6.0 ||
		st["escalation_grace_minutes"] != 90.0 || len(st["cones"].([]any)) != 2 {
		t.Fatalf("after update = %v", st)
	}
	opts := km.mustDo("GET", "/api/options", nil, http.StatusOK)
	if opts["min_notice_hours"] != 24.0 {
		t.Fatalf("options didn't pick up the change: %v", opts)
	}

	for name, bad := range map[string]func(map[string]any){
		"timezone":      func(m map[string]any) { m["timezone"] = "Mars/Olympus" },
		"dup cone":      func(m map[string]any) { m["cones"] = []string{"6", "6"} },
		"no types":      func(m map[string]any) { m["firing_types"] = []string{} },
		"threshold":     func(m map[string]any) { m["underfilled_threshold_pct"] = 150 },
		"reminder hour": func(m map[string]any) { m["day_reminder_hour"] = 24 },
		"escalation":    func(m map[string]any) { m["escalation_grace_minutes"] = -1 },
		"days":          func(m map[string]any) { m["public_page_days_ahead"] = 0 },
	} {
		m := map[string]any{}
		for k, v := range st {
			m[k] = v
		}
		bad(m)
		if code, out := admin.do("PUT", "/api/admin/settings", m); code != http.StatusBadRequest {
			t.Errorf("%s: got %d %v", name, code, out)
		}
	}

	// Kilns: add one, edit dimensions, can't deactivate the last active kiln.
	first := int64(got["kilns"].([]any)[0].(map[string]any)["id"].(float64))
	k := admin.mustDo("POST", "/api/admin/kilns", map[string]any{"name": "Test kiln", "active": true, "shelf_count": 4,
		"share_per_shelf_pct": 25}, http.StatusCreated)
	second := int64(k["id"].(float64))
	km.mustDo("POST", "/api/admin/kilns", map[string]any{"name": "x", "active": true}, http.StatusForbidden)
	admin.mustDo("PUT", fmt.Sprintf("/api/admin/kilns/%d", first), map[string]any{"name": "Big kiln", "active": false,
		"interior_width_in": 23.5}, http.StatusOK)
	admin.mustDo("PUT", fmt.Sprintf("/api/admin/kilns/%d", second), map[string]any{"name": "Test kiln", "active": false},
		http.StatusConflict)
	admin.mustDo("PUT", "/api/admin/kilns/999999", map[string]any{"name": "x", "active": true}, http.StatusNotFound)
	admin.mustDo("PUT", fmt.Sprintf("/api/admin/kilns/%d", first), map[string]any{"name": "Big kiln", "active": true}, http.StatusOK)

	// Profiles: per kiln, active ones appear in the proposal options.
	p := admin.mustDo("POST", "/api/admin/profiles", map[string]any{
		"kiln_id": first, "name": "Cone 6 Glaze — standard", "firing_type": "glaze", "cone": "6",
		"firing_minutes": 600, "cooldown_minutes": 840, "active": true,
	}, http.StatusCreated)
	profileID := int64(p["id"].(float64))
	opts = km.mustDo("GET", "/api/options", nil, http.StatusOK)
	if n := len(opts["profiles"].([]any)); n != 1 {
		t.Fatalf("options profiles = %v", opts["profiles"])
	}
	p = admin.mustDo("PUT", fmt.Sprintf("/api/admin/profiles/%d", profileID), map[string]any{
		"kiln_id": first, "name": "Cone 6 Glaze — slow", "firing_type": "glaze", "cone": "6",
		"firing_minutes": 720, "cooldown_minutes": 840, "active": true,
	}, http.StatusOK)
	if p["name"] != "Cone 6 Glaze — slow" {
		t.Fatalf("profile update = %v", p)
	}
	admin.mustDo("DELETE", fmt.Sprintf("/api/admin/profiles/%d", profileID), nil, http.StatusOK)
	opts = km.mustDo("GET", "/api/options", nil, http.StatusOK)
	if n := len(opts["profiles"].([]any)); n != 0 {
		t.Fatalf("deactivated profile should not be offered: %v", opts["profiles"])
	}
}
