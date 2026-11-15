package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/spencercornish/kilnlab/internal/config"
)

type outboxRow struct {
	kind, platform string
	userID         int64
	thread, text   string
}

// drainOutbox returns and deletes all queued notifications.
func drainOutbox(t *testing.T, pool *pgxpool.Pool) []outboxRow {
	t.Helper()
	rows, err := pool.Query(context.Background(), `DELETE FROM notification_outbox RETURNING kind::text, platform::text, payload`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []outboxRow
	for rows.Next() {
		var r outboxRow
		var payload []byte
		if err := rows.Scan(&r.kind, &r.platform, &payload); err != nil {
			t.Fatal(err)
		}
		var p struct {
			UserID int64  `json:"user_id"`
			Thread string `json:"thread"`
			Text   string `json:"text"`
		}
		_ = json.Unmarshal(payload, &p)
		r.userID, r.thread, r.text = p.UserID, p.Thread, p.Text
		out = append(out, r)
	}
	return out
}

func dmRecipients(rows []outboxRow) []int64 {
	var ids []int64
	for _, r := range rows {
		if r.kind == "dm" {
			ids = append(ids, r.userID)
		}
	}
	slices.Sort(ids)
	return ids
}

func threadOf(rows []outboxRow) string {
	for _, r := range rows {
		if r.kind == "channel_upsert" && r.thread != "" {
			return r.thread
		}
	}
	return ""
}

func TestActionsEnqueueNotifications(t *testing.T) {
	e, pool := newFiringEnvPool(t, func(c *config.Config) { c.NotificationsDryRun = true })
	alice := e.login("Alice", false)
	bob := e.login("Bob", false)
	carol := e.login("Carol", false)
	km := e.login("Kim", true)
	id := func(c *apiClient) int64 {
		return int64(jget(c.mustDo("GET", "/api/me", nil, http.StatusOK), "id").(float64))
	}
	aliceID, bobID, carolID, kimID := id(alice), id(bob), id(carol), id(km)

	d := alice.mustDo("POST", "/api/firings", map[string]any{"firing_type": "glaze", "cone": "6",
		"scheduled_date": dayOf(e.now.Add(72 * time.Hour)), "description": "mugs", "share_pct": 30}, http.StatusCreated)
	base := fmt.Sprintf("/api/firings/%d", int64(jget(d, "firing", "id").(float64)))
	rows := drainOutbox(t, pool)
	if len(rows) != 2 || rows[0].kind != "channel_upsert" { // one main message per platform
		t.Fatalf("on create: %+v", rows)
	}

	// A new request refreshes the main message and DMs every kiln master.
	d = bob.mustDo("POST", base+"/requests", map[string]any{"description": "bowls", "share_pct": 20, "cone": "6"}, http.StatusOK)
	bobReq := int64(jget(d, "my_request", "id").(float64))
	rows = drainOutbox(t, pool)
	if got := dmRecipients(rows); !slices.Equal(got, []int64{kimID}) {
		t.Fatalf("request DMs = %v, want the kiln master", got)
	}
	for _, r := range rows {
		if r.kind == "dm" && !strings.Contains(r.text, "Bob requested 20%") {
			t.Fatalf("request DM text = %q", r.text)
		}
	}
	d = carol.mustDo("POST", base+"/requests", map[string]any{"description": "cups", "share_pct": 20, "cone": "6"}, http.StatusOK)
	carolReq := int64(jget(d, "my_request", "id").(float64))
	if got := dmRecipients(drainOutbox(t, pool)); !slices.Equal(got, []int64{kimID}) {
		t.Fatalf("second request DMs = %v", got)
	}

	// Approve → DM the requester only.
	km.mustDo("POST", fmt.Sprintf("%s/requests/%d/decide", base, bobReq), map[string]any{"decision": "approve", "note": "fits"}, http.StatusOK)
	rows = drainOutbox(t, pool)
	if got := dmRecipients(rows); !slices.Equal(got, []int64{bobID}) {
		t.Fatalf("approve DMs = %v", got)
	}

	// Hold → everyone in the firing (pending + approved + proposer).
	km.mustDo("POST", base+"/hold", map[string]any{"reason": "waiting"}, http.StatusOK)
	if got := dmRecipients(drainOutbox(t, pool)); !slices.Equal(got, []int64{aliceID, bobID, carolID}) {
		t.Fatalf("hold DMs = %v", got)
	}
	km.mustDo("POST", base+"/release", map[string]any{}, http.StatusOK)
	if got := dmRecipients(drainOutbox(t, pool)); len(got) != 0 {
		t.Fatalf("plain release shouldn't DM: %v", got)
	}

	// Reschedule → thread reply + DMs.
	km.mustDo("POST", base+"/reschedule", map[string]any{"scheduled_date": dayOf(e.now.Add(96 * time.Hour))}, http.StatusOK)
	rows = drainOutbox(t, pool)
	if threadOf(rows) == "" || len(dmRecipients(rows)) != 3 {
		t.Fatalf("reschedule: thread %q, DMs %v", threadOf(rows), dmRecipients(rows))
	}

	// Decline Carol, then run the firing: start → thread; ready → DMs only to approved + proposer.
	km.mustDo("POST", fmt.Sprintf("%s/requests/%d/decide", base, carolReq), map[string]any{"decision": "decline"}, http.StatusOK)
	km.mustDo("POST", base+"/confirm", nil, http.StatusOK)
	drainOutbox(t, pool)
	km.mustDo("POST", base+"/status", map[string]any{"to": "firing", "program": "Glaze", "estimated_duration_minutes": 600}, http.StatusOK)
	if th := threadOf(drainOutbox(t, pool)); th == "" {
		t.Fatal("start should post a thread reply")
	}
	km.mustDo("POST", base+"/status", map[string]any{"to": "completed"}, http.StatusOK)
	rows = drainOutbox(t, pool)
	if got := dmRecipients(rows); !slices.Equal(got, []int64{aliceID, bobID}) || threadOf(rows) == "" {
		t.Fatalf("ready: DMs %v, thread %q", got, threadOf(rows))
	}

	// The actor isn't DMed: Alice (proposer) cancelling isn't possible, so check a KM-proposed firing.
	d = km.mustDo("POST", "/api/firings", map[string]any{"firing_type": "bisque", "cone": "04",
		"scheduled_date": dayOf(e.now.Add(72 * time.Hour)), "description": "x", "share_pct": 10}, http.StatusCreated)
	base2 := fmt.Sprintf("/api/firings/%d", int64(jget(d, "firing", "id").(float64)))
	bob.mustDo("POST", base2+"/requests", map[string]any{"description": "y", "share_pct": 10, "cone": "04"}, http.StatusOK)
	drainOutbox(t, pool)
	km.mustDo("POST", base2+"/cancel", map[string]any{"reason": "kiln broken"}, http.StatusOK)
	if got := dmRecipients(drainOutbox(t, pool)); !slices.Equal(got, []int64{bobID}) {
		t.Fatalf("cancel DMs = %v (kiln master proposer should be skipped)", got)
	}
}
