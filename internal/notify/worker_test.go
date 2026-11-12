package notify

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/spencercornish/kilnlab/internal/db"
	"github.com/spencercornish/kilnlab/internal/firing"
)

// fakeAPI records requests and answers with canned or scripted responses.
type fakeAPI struct {
	mu       sync.Mutex
	calls    []string // "METHOD path"
	bodies   []map[string]any
	failNext []int // status codes to return for the next calls
	handler  func(w http.ResponseWriter, r *http.Request, body map[string]any)
}

func (f *fakeAPI) serve(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		f.mu.Lock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		f.bodies = append(f.bodies, body)
		var fail int
		if len(f.failNext) > 0 {
			fail, f.failNext = f.failNext[0], f.failNext[1:]
		}
		f.mu.Unlock()
		if fail != 0 {
			w.WriteHeader(fail)
			_, _ = w.Write([]byte(`{"message":"scripted failure","code":50007}`))
			return
		}
		f.handler(w, r, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (f *fakeAPI) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.calls
	f.calls = nil
	return c
}

func discordFake() *fakeAPI {
	return &fakeAPI{handler: func(w http.ResponseWriter, r *http.Request, _ map[string]any) {
		switch {
		case r.URL.Path == "/users/@me/channels":
			_, _ = w.Write([]byte(`{"id":"dm1"}`))
		case strings.HasSuffix(r.URL.Path, "/threads"):
			_, _ = w.Write([]byte(`{"id":"thread1"}`))
		default:
			_, _ = w.Write([]byte(`{"id":"msg1"}`))
		}
	}}
}

func slackFake() *fakeAPI {
	return &fakeAPI{handler: func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		_, _ = w.Write([]byte(`{"ok":true,"channel":"C123","ts":"111.222"}`))
	}}
}

type env struct {
	pool    *pgxpool.Pool
	q       *db.Queries
	worker  *Worker
	cfg     Config
	discord *fakeAPI
	slack   *fakeAPI
	now     time.Time
	userID  int64 // discord user (proposer)
	slackID int64 // slack user
	firing  db.Firing
}

func newEnv(t *testing.T) *env {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run integration tests")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, sql := range []string{
		`TRUNCATE users, accounts, sessions, oauth_states, notification_outbox RESTART IDENTITY CASCADE`,
		`DELETE FROM kilns WHERE id <> (SELECT min(id) FROM kilns)`,
		`UPDATE settings SET discord_channel_id = 'D-CHAN', slack_channel_id = 'S-CHAN'`,
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE settings SET discord_channel_id = '', slack_channel_id = ''`)
	})

	e := &env{pool: pool, q: db.New(pool), discord: discordFake(), slack: slackFake(),
		// A minute ahead of the DB clock, so freshly enqueued rows are due.
		now: time.Now().UTC().Add(time.Minute)}
	e.cfg = Config{BaseURL: "https://kiln.example", DiscordBotToken: "dt", SlackBotToken: "st"}
	e.worker = NewWorker(pool, e.cfg, slog.New(slog.DiscardHandler), WorkerOptions{
		DiscordBaseURL: e.discord.serve(t).URL, SlackBaseURL: e.slack.serve(t).URL,
		Now: func() time.Time { return e.now },
	})

	mk := func(name, community, provider, pid string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `WITH u AS (INSERT INTO users (display_name, community) VALUES ($1, $2) RETURNING id)
			INSERT INTO accounts (user_id, provider, provider_user_id, provider_team_or_guild_id)
			SELECT id, $3, $4, '' FROM u RETURNING user_id`, name, community, provider, pid).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	e.userID = mk("Cody", "artlab", "discord", "D-USER")
	e.slackID = mk("Bea", "bms", "slack", "U-BEA")
	e.firing, err = e.q.CreateFiring(ctx, db.CreateFiringParams{
		KilnID: 1, ProposedByUserID: e.userID, FiringType: "glaze", Cone: "6", ScheduledDate: firing.DayOf(e.now.Add(72*time.Hour), time.UTC),
		ProposerDescription: "mugs", ProposerSharePct: 40, ProposerCone: "6", RequestsCloseAt: e.now.Add(60 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *env) enqueue(t *testing.T, b *Batch, actor int64) {
	t.Helper()
	if err := Enqueue(context.Background(), e.q, e.cfg, e.firing.ID, actor, b); err != nil {
		t.Fatal(err)
	}
}

func (e *env) run(t *testing.T) int {
	t.Helper()
	n, err := e.worker.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func equal(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("%s calls = %v, want %v", what, got, want)
	}
}

func TestChannelMessageCreatedThenEditedInPlace(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	e.enqueue(t, &Batch{}, e.userID)
	if n := e.run(t); n != 2 {
		t.Fatalf("processed %d rows, want 2 (discord + slack)", n)
	}
	equal(t, "discord", e.discord.take(), []string{"POST /channels/D-CHAN/messages"})
	equal(t, "slack", e.slack.take(), []string{"POST /chat.postMessage"})

	// Nothing changed: no edits (render-hash dedupe).
	e.enqueue(t, &Batch{}, e.userID)
	e.run(t)
	equal(t, "discord unchanged", e.discord.take(), nil)
	equal(t, "slack unchanged", e.slack.take(), nil)

	// A change edits the same message.
	if _, err := e.q.SetFiringStatus(ctx, db.SetFiringStatusParams{ID: e.firing.ID, Status: firing.Confirmed}); err != nil {
		t.Fatal(err)
	}
	e.enqueue(t, &Batch{}, e.userID)
	e.run(t)
	equal(t, "discord edit", e.discord.take(), []string{"PATCH /channels/D-CHAN/messages/msg1"})
	equal(t, "slack edit", e.slack.take(), []string{"POST /chat.update"})
	if body := e.slack.bodies[len(e.slack.bodies)-1]; body["channel"] != "C123" || body["ts"] != "111.222" {
		t.Errorf("chat.update body = %v", body)
	}

	// Thread replies: Discord creates the thread once, then reuses it.
	e.enqueue(t, &Batch{Thread: "🔥 Firing started"}, e.userID)
	e.run(t)
	equal(t, "discord thread", e.discord.take(), []string{"POST /channels/D-CHAN/messages/msg1/threads", "POST /channels/thread1/messages"})
	equal(t, "slack thread", e.slack.take(), []string{"POST /chat.postMessage"})
	if body := e.slack.bodies[len(e.slack.bodies)-1]; body["thread_ts"] != "111.222" {
		t.Errorf("slack thread body = %v", body)
	}
	e.enqueue(t, &Batch{Thread: "✅ Ready"}, e.userID)
	e.run(t)
	equal(t, "discord thread reuse", e.discord.take(), []string{"POST /channels/thread1/messages"})
	e.slack.take()
}

func TestDMsGoToSignInPlatformAndSkipActor(t *testing.T) {
	e := newEnv(t)
	b := &Batch{}
	b.DM([]int64{e.userID, e.slackID, e.slackID}, "hello")
	e.enqueue(t, b, 0)
	e.run(t)
	equal(t, "discord", e.discord.take(), []string{"POST /channels/D-CHAN/messages", "POST /users/@me/channels", "POST /channels/dm1/messages"})
	slackCalls := e.slack.take()
	if len(slackCalls) != 2 { // card + one DM (duplicate user collapsed)
		t.Fatalf("slack calls = %v", slackCalls)
	}
	if e.slack.bodies[len(e.slack.bodies)-1]["channel"] != "U-BEA" {
		t.Errorf("slack DM body = %v", e.slack.bodies[len(e.slack.bodies)-1])
	}

	// The actor isn't DMed about their own action.
	b = &Batch{}
	b.DM([]int64{e.userID}, "you did a thing")
	e.enqueue(t, b, e.userID)
	e.run(t)
	for _, c := range e.discord.take() {
		if c == "POST /users/@me/channels" {
			t.Error("actor was DMed")
		}
	}
}

func outboxRows(t *testing.T, e *env) []db.NotificationOutbox {
	rows, err := e.q.ListOutbox(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestRetriesWithBackoffThenSucceeds(t *testing.T) {
	e := newEnv(t)
	e.cfg.SlackBotToken = "" // Discord only
	e.discord.failNext = []int{http.StatusBadGateway}
	e.enqueue(t, &Batch{}, e.userID)
	e.run(t)

	rows := outboxRows(t, e)
	if len(rows) != 1 || rows[0].Status != db.OutboxStatusPending || rows[0].Attempts != 1 || rows[0].LastError == nil {
		t.Fatalf("after failure: %+v", rows)
	}
	if !rows[0].NextAttemptAt.After(e.now) {
		t.Fatal("retry should be scheduled in the future")
	}
	if n := e.run(t); n != 0 {
		t.Fatalf("retried too early (%d)", n)
	}
	e.now = e.now.Add(Backoff(1) + time.Second)
	e.run(t)
	if rows = outboxRows(t, e); rows[0].Status != db.OutboxStatusSent || rows[0].Attempts != 2 {
		t.Fatalf("after retry: %+v", rows[0])
	}
}

func TestPermanentFailureStopsRetrying(t *testing.T) {
	e := newEnv(t)
	e.cfg.SlackBotToken = ""
	b := &Batch{}
	b.DM([]int64{e.userID}, "hi")
	if err := Enqueue(context.Background(), e.q, e.cfg, 0, 0, b); err != nil { // DM only
		t.Fatal(err)
	}
	e.discord.failNext = []int{http.StatusForbidden} // "Cannot send messages to this user"
	e.run(t)
	rows := outboxRows(t, e)
	if len(rows) != 1 || rows[0].Status != db.OutboxStatusFailed || rows[0].Attempts != 1 {
		t.Fatalf("cannot-DM should fail permanently: %+v", rows)
	}
}

func TestGivesUpAfterMaxAttempts(t *testing.T) {
	e := newEnv(t)
	e.cfg.SlackBotToken = ""
	e.enqueue(t, &Batch{}, e.userID)
	for i := 0; i < MaxAttempts; i++ {
		e.discord.failNext = []int{http.StatusInternalServerError}
		e.run(t)
		e.now = e.now.Add(2 * time.Hour)
	}
	if rows := outboxRows(t, e); rows[0].Status != db.OutboxStatusFailed || rows[0].Attempts != MaxAttempts {
		t.Fatalf("after %d failures: %+v", MaxAttempts, rows[0])
	}
}

func TestDeletedMessageIsReposted(t *testing.T) {
	e := newEnv(t)
	e.cfg.SlackBotToken = ""
	e.enqueue(t, &Batch{}, e.userID)
	e.run(t)
	e.discord.take()

	ctx := context.Background()
	if _, err := e.q.SetFiringStatus(ctx, db.SetFiringStatusParams{ID: e.firing.ID, Status: firing.Confirmed}); err != nil {
		t.Fatal(err)
	}
	e.discord.handler = func(w http.ResponseWriter, r *http.Request, _ map[string]any) {
		if r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Unknown Message","code":10008}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"msg2"}`))
	}
	e.enqueue(t, &Batch{}, e.userID)
	e.run(t)
	equal(t, "repost", e.discord.take(), []string{"PATCH /channels/D-CHAN/messages/msg1", "POST /channels/D-CHAN/messages"})
	msg, err := e.q.GetNotificationMessage(ctx, db.GetNotificationMessageParams{FiringID: e.firing.ID, Platform: db.AuthProviderDiscord})
	if err != nil || msg.MessageID != "msg2" {
		t.Fatalf("stored message = %+v, %v", msg, err)
	}
}

func TestDryRunLogsWithoutTokens(t *testing.T) {
	e := newEnv(t)
	e.cfg = Config{BaseURL: "https://kiln.example", DryRun: true}
	e.worker = NewWorker(e.pool, e.cfg, slog.New(slog.DiscardHandler), WorkerOptions{Now: func() time.Time { return e.now }})
	b := &Batch{Thread: "hi"}
	b.DM([]int64{e.slackID}, "dm")
	e.enqueue(t, b, 0)
	if n := e.run(t); n != 3 {
		t.Fatalf("dry run processed %d, want 3", n)
	}
	for _, r := range outboxRows(t, e) {
		if r.Status != db.OutboxStatusSent {
			t.Errorf("row %d status %s", r.ID, r.Status)
		}
	}
	if len(e.discord.take())+len(e.slack.take()) != 0 {
		t.Error("dry run must not call the APIs")
	}
}
