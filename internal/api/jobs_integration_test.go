package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spencercornish/kilnlab/internal/config"
	"github.com/spencercornish/kilnlab/internal/firing"
	"github.com/spencercornish/kilnlab/internal/jobs"
)

// Spec §14 step 9 and the §10 jobs: each fires exactly once, and reschedule /
// reopen re-arm them.
func TestScheduledJobs(t *testing.T) {
	e, pool := newFiringEnvPool(t, func(c *config.Config) { c.NotificationsDryRun = true })
	alice := e.login("Alice", false)
	km := e.login("Kim", true)
	id := func(c *apiClient) int64 {
		return int64(jget(c.mustDo("GET", "/api/me", nil, http.StatusOK), "id").(float64))
	}
	kimID := id(km)

	runner := jobs.NewRunner(pool, NotifyConfig(&config.Config{AppBaseURL: "http://k", NotificationsDryRun: true}), nil,
		slog.New(slog.DiscardHandler))
	runner.Now = func() time.Time { e.mu.Lock(); defer e.mu.Unlock(); return e.now }
	tick := func(wantActed int) []outboxRow {
		t.Helper()
		n, err := runner.Tick(context.Background())
		if err != nil || n != wantActed {
			t.Fatalf("tick: acted %d (want %d), err %v", n, wantActed, err)
		}
		return drainOutbox(t, pool)
	}
	dmText := func(rows []outboxRow) string {
		for _, r := range rows {
			if r.kind == "dm" {
				return r.text
			}
		}
		return ""
	}

	dayRef := e.now.Add(72 * time.Hour) // a time on the firing's day
	dayStartAt := firing.DayStart(firing.DayOf(dayRef, studioLoc), studioLoc)
	cutoffAt := dayStartAt.Add(-12 * time.Hour) // the default request cutoff
	d := alice.mustDo("POST", "/api/firings", map[string]any{"firing_type": "glaze", "cone": "6",
		"scheduled_date": dayOf(dayRef), "description": "mugs", "share_pct": 30}, http.StatusCreated)
	base := fmt.Sprintf("/api/firings/%d", int64(jget(d, "firing", "id").(float64)))
	drainOutbox(t, pool)
	tick(0)

	// Past the 12h cutoff: requests close, kiln masters hear it's under-filled and unconfirmed.
	e.advance(cutoffAt.Add(time.Minute).Sub(e.now))
	rows := tick(1)
	if got := dmRecipients(rows); !slices.Equal(got, []int64{kimID}) {
		t.Fatalf("under-filled DMs = %v", got)
	}
	if txt := dmText(rows); !strings.Contains(txt, "only 30% full") || !strings.Contains(txt, "not confirmed") {
		t.Fatalf("under-filled text = %q", txt)
	}
	if len(rows) != 3 { // + main message on both platforms ("Requests closed")
		t.Fatalf("rows = %+v", rows)
	}
	tick(0) // exactly once

	// Confirm and reopen requests until the start: re-armed.
	km.mustDo("POST", base+"/confirm", nil, http.StatusOK)
	km.mustDo("PUT", base+"/requests-window", map[string]any{"closed": false}, http.StatusOK)
	drainOutbox(t, pool)
	tick(0)

	// Start passes: requests close again (still under-filled); missed start waits for the 2h grace.
	e.advance(dayStartAt.Add(time.Minute).Sub(e.now))
	if txt := dmText(tick(1)); !strings.Contains(txt, "only 30% full") || strings.Contains(txt, "not confirmed") {
		t.Fatalf("second close text = %q", txt)
	}
	// A start is missed only once its day has ended, plus the 2h grace.
	missedAt := firing.DayEnd(firing.DayOf(dayRef, studioLoc), studioLoc).Add(2 * time.Hour)
	e.advance(missedAt.Add(time.Minute).Sub(e.now))
	rows = tick(1)
	if got := dmRecipients(rows); !slices.Equal(got, []int64{kimID}) || !strings.Contains(dmText(rows), "hasn't been marked started") {
		t.Fatalf("missed start: %v %q", got, dmText(rows))
	}
	for _, r := range rows {
		if r.kind != "dm" {
			t.Fatalf("a reminder shouldn't touch the main message: %+v", r)
		}
	}
	tick(0)

	// Started with a 1h program; unload overdue after est. + 3h grace → kiln masters.
	km.mustDo("POST", base+"/status", map[string]any{"to": "firing", "program": "Fast", "estimated_duration_minutes": 60}, http.StatusOK)
	drainOutbox(t, pool)
	e.advance(4 * time.Hour)
	tick(0)
	e.advance(2 * time.Minute)
	rows = tick(1)
	if got := dmRecipients(rows); !slices.Equal(got, []int64{kimID}) {
		t.Fatalf("missed unload DMs = %v", got)
	}
	tick(0)

	// The timeline shows what the system did.
	d = alice.mustDo("GET", base, nil, http.StatusOK)
	alerts := 0
	for _, ev := range d["events"].([]any) {
		if m := ev.(map[string]any); m["type"] == "alert_sent" && m["source"] == "system" {
			alerts++
		}
	}
	if alerts != 4 {
		t.Fatalf("alert events = %d, want 4", alerts)
	}
}

// A firing a kiln master schedules inside the cutoff closes quietly.
func TestJobsShortNoticeFiring(t *testing.T) {
	e, pool := newFiringEnvPool(t, func(c *config.Config) { c.NotificationsDryRun = true })
	km := e.login("Kim", true)
	km.mustDo("POST", "/api/firings", map[string]any{"firing_type": "bisque", "cone": "04",
		"scheduled_date": dayOf(e.now.Add(3 * time.Hour)), "description": "tiles", "share_pct": 10}, http.StatusCreated)
	drainOutbox(t, pool)
	runner := jobs.NewRunner(pool, NotifyConfig(&config.Config{NotificationsDryRun: true}), nil, slog.New(slog.DiscardHandler))
	runner.Now = func() time.Time { return e.now.Add(time.Minute) }
	if n, err := runner.Tick(context.Background()); err != nil || n != 1 {
		t.Fatalf("acted %d, %v", n, err)
	}
	if got := dmRecipients(drainOutbox(t, pool)); len(got) != 0 {
		t.Fatalf("no alerts expected, got DMs to %v", got)
	}
}
