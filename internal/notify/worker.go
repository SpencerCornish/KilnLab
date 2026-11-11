package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/spencercornish/kilnlab/internal/db"
	"github.com/spencercornish/kilnlab/internal/firing"
)

// Worker delivers notification_outbox rows. Run one per process; rows are
// claimed with SKIP LOCKED so several processes would also be safe, though
// edits to the same firing's message are only ordered within one worker.
type Worker struct {
	pool     *pgxpool.Pool
	cfg      Config
	discord  *Discord // nil without a bot token
	slack    *Slack
	log      *slog.Logger
	now      func() time.Time
	interval time.Duration
	kick     chan struct{}
}

// WorkerOptions are for tests: point the clients at fake servers.
type WorkerOptions struct {
	DiscordBaseURL string
	SlackBaseURL   string
	Now            func() time.Time
}

func NewWorker(pool *pgxpool.Pool, cfg Config, log *slog.Logger, opts WorkerOptions) *Worker {
	if log == nil {
		log = slog.Default()
	}
	w := &Worker{pool: pool, cfg: cfg, log: log.With("component", "notify"), now: time.Now,
		interval: 10 * time.Second, kick: make(chan struct{}, 1)}
	if opts.Now != nil {
		w.now = opts.Now
	}
	if cfg.DiscordBotToken != "" {
		w.discord = NewDiscord(cfg.DiscordBotToken, opts.DiscordBaseURL)
	}
	if cfg.SlackBotToken != "" {
		w.slack = NewSlack(cfg.SlackBotToken, opts.SlackBaseURL)
	}
	return w
}

// Kick asks the worker to look for work now (after a change commits).
func (w *Worker) Kick() {
	if w == nil {
		return
	}
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

// Run delivers due rows until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		if _, err := w.RunOnce(ctx); err != nil && ctx.Err() == nil {
			w.log.Error("outbox run failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-w.kick:
		}
	}
}

// RunOnce delivers every row that is due now and returns how many it handled.
func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	n := 0
	for ctx.Err() == nil {
		did, err := w.processOne(ctx)
		if err != nil || !did {
			return n, err
		}
		n++
	}
	return n, nil
}

func (w *Worker) processOne(ctx context.Context) (bool, error) {
	found := false
	err := db.InTx(ctx, w.pool, func(q *db.Queries) error {
		row, err := q.ClaimDueOutbox(ctx, w.now())
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		sendErr := w.deliver(ctx, q, row)
		if sendErr == nil {
			return q.MarkOutboxSent(ctx, row.ID)
		}
		attempts := int(row.Attempts) + 1
		msg := sendErr.Error()
		se := asSendError(sendErr)
		if (se != nil && se.Permanent) || attempts >= MaxAttempts {
			w.log.Warn("notification failed", "id", row.ID, "kind", row.Kind, "platform", row.Platform, "attempts", attempts, "error", msg)
			return q.MarkOutboxFailed(ctx, db.MarkOutboxFailedParams{ID: row.ID, LastError: &msg})
		}
		wait := Backoff(attempts)
		if se != nil && se.RetryAfter > wait {
			wait = se.RetryAfter
		}
		w.log.Info("notification will retry", "id", row.ID, "attempts", attempts, "in", wait.String(), "error", msg)
		return q.MarkOutboxRetry(ctx, db.MarkOutboxRetryParams{ID: row.ID, LastError: &msg, NextAttemptAt: w.now().Add(wait)})
	})
	return found, err
}

func permanent(format string, args ...any) error {
	return &SendError{Msg: fmt.Sprintf(format, args...), Permanent: true}
}

func (w *Worker) deliver(ctx context.Context, q *db.Queries, row db.NotificationOutbox) error {
	switch row.Kind {
	case db.OutboxKindChannelUpsert:
		var p upsertPayload
		if err := json.Unmarshal(row.Payload, &p); err != nil {
			return permanent("bad payload: %v", err)
		}
		return w.upsert(ctx, q, row.Platform, p)
	case db.OutboxKindDm:
		var p dmPayload
		if err := json.Unmarshal(row.Payload, &p); err != nil {
			return permanent("bad payload: %v", err)
		}
		return w.dm(ctx, q, row.Platform, p)
	}
	return permanent("unknown kind %q", row.Kind)
}

// card renders the firing's main message from its current state.
func (w *Worker) card(ctx context.Context, q *db.Queries, f db.Firing, st db.Setting) (Card, error) {
	name, err := q.GetUserName(ctx, f.ProposedByUserID)
	if err != nil {
		return Card{}, err
	}
	approved, err := q.ApprovedShares(ctx, f.ID)
	if err != nil {
		return Card{}, err
	}
	kilns, err := q.ListKilns(ctx)
	if err != nil {
		return Card{}, err
	}
	in := CardInput{
		Firing: f, ProposerName: name, ApprovedCount: len(approved), Now: w.now(), BaseURL: w.cfg.BaseURL,
		Fullness: firing.ComputeFullness(f.ProposerSharePct, approved, f.FullnessOverridePct), Loc: location(st),
	}
	active := 0
	for _, k := range kilns {
		if k.Active {
			active++
		}
		if k.ID == f.KilnID {
			in.KilnName = k.Name
		}
	}
	if active < 2 {
		in.KilnName = ""
	}
	return RenderCard(in), nil
}

func location(st db.Setting) *time.Location {
	loc, err := time.LoadLocation(st.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func (w *Worker) upsert(ctx context.Context, q *db.Queries, platform db.AuthProvider, p upsertPayload) error {
	f, err := q.GetFiring(ctx, p.FiringID)
	if errors.Is(err, pgx.ErrNoRows) {
		return permanent("firing %d no longer exists", p.FiringID)
	} else if err != nil {
		return err
	}
	st, err := q.GetSettings(ctx)
	if err != nil {
		return err
	}
	card, err := w.card(ctx, q, f, st)
	if err != nil {
		return err
	}
	if w.cfg.DryRun {
		w.log.Info("notification (dry run)", "kind", "channel_upsert", "platform", platform, "firing", f.ID,
			"message", card.Text(), "thread", p.Thread)
		return nil
	}
	channel := w.cfg.Channel(st, platform)
	if channel == "" {
		return permanent("no %s channel configured", platform)
	}

	msg, err := q.GetNotificationMessage(ctx, db.GetNotificationMessageParams{FiringID: f.ID, Platform: platform})
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	save := func() error {
		return q.UpsertNotificationMessage(ctx, db.UpsertNotificationMessageParams{
			FiringID: f.ID, Platform: platform, ChannelID: msg.ChannelID, MessageID: msg.MessageID,
			ThreadID: msg.ThreadID, LastRenderedHash: msg.LastRenderedHash,
		})
	}
	hash := card.Hash()

	switch platform {
	case db.AuthProviderDiscord:
		if w.discord == nil {
			return permanent("DISCORD_BOT_TOKEN is not set")
		}
		post := func() error {
			id, err := w.discord.PostCard(ctx, channel, card)
			if err != nil {
				return err
			}
			msg = db.NotificationMessage{ChannelID: channel, MessageID: id, LastRenderedHash: hash}
			return save()
		}
		switch {
		case !exists:
			err = post()
		case msg.LastRenderedHash != hash:
			if err = w.discord.EditCard(ctx, msg.ChannelID, msg.MessageID, card); err == nil {
				msg.LastRenderedHash = hash
				err = save()
			} else if se := asSendError(err); se != nil && se.NotFound {
				err = post() // someone deleted it; start a fresh one
			}
		}
		if err != nil {
			return err
		}
		if p.Thread == "" {
			return nil
		}
		if msg.ThreadID == nil {
			tid, err := w.discord.StartThread(ctx, msg.ChannelID, msg.MessageID, Title(f))
			if err != nil {
				return err
			}
			msg.ThreadID = &tid
			if err := save(); err != nil {
				return err
			}
		}
		return w.discord.PostText(ctx, *msg.ThreadID, p.Thread)

	case db.AuthProviderSlack:
		if w.slack == nil {
			return permanent("SLACK_BOT_TOKEN is not set")
		}
		post := func() error {
			ch, ts, err := w.slack.PostCard(ctx, channel, card)
			if err != nil {
				return err
			}
			msg = db.NotificationMessage{ChannelID: ch, MessageID: ts, LastRenderedHash: hash}
			return save()
		}
		switch {
		case !exists:
			err = post()
		case msg.LastRenderedHash != hash:
			if err = w.slack.UpdateCard(ctx, msg.ChannelID, msg.MessageID, card); err == nil {
				msg.LastRenderedHash = hash
				err = save()
			} else if se := asSendError(err); se != nil && se.NotFound {
				err = post()
			}
		}
		if err != nil || p.Thread == "" {
			return err
		}
		return w.slack.PostText(ctx, msg.ChannelID, p.Thread, msg.MessageID)
	}
	return permanent("unknown platform %q", platform)
}

func (w *Worker) dm(ctx context.Context, q *db.Queries, platform db.AuthProvider, p dmPayload) error {
	accounts, err := q.GetUserAccounts(ctx, p.UserID)
	if err != nil {
		return err
	}
	var to string
	for _, a := range accounts {
		if a.Provider == platform {
			to = a.ProviderUserID
		}
	}
	if to == "" {
		return permanent("user %d has no %s account", p.UserID, platform)
	}
	if w.cfg.DryRun {
		w.log.Info("notification (dry run)", "kind", "dm", "platform", platform, "user", p.UserID, "message", p.Text)
		return nil
	}
	switch platform {
	case db.AuthProviderDiscord:
		if w.discord == nil {
			return permanent("DISCORD_BOT_TOKEN is not set")
		}
		return w.discord.DM(ctx, to, p.Text)
	case db.AuthProviderSlack:
		if w.slack == nil {
			return permanent("SLACK_BOT_TOKEN is not set")
		}
		return w.slack.PostText(ctx, to, p.Text, "")
	}
	return permanent("unknown platform %q", platform)
}
