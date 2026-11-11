package notify

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/spencercornish/kilnlab/internal/db"
)

// Config is what notify needs from the app configuration.
type Config struct {
	BaseURL                string
	DiscordBotToken        string
	SlackBotToken          string
	DiscordChannelFallback string // DISCORD_CHANNEL_ID, used when settings has none
	SlackChannelFallback   string
	DryRun                 bool // log instead of sending; works without tokens
}

var platforms = []db.AuthProvider{db.AuthProviderDiscord, db.AuthProviderSlack}

func (c Config) enabled(p db.AuthProvider) bool {
	if c.DryRun {
		return true
	}
	switch p {
	case db.AuthProviderDiscord:
		return c.DiscordBotToken != ""
	case db.AuthProviderSlack:
		return c.SlackBotToken != ""
	}
	return false
}

// Channel is the notification channel for a platform: settings, else env.
func (c Config) Channel(st db.Setting, p db.AuthProvider) string {
	switch p {
	case db.AuthProviderDiscord:
		if st.DiscordChannelID != "" {
			return st.DiscordChannelID
		}
		return c.DiscordChannelFallback
	case db.AuthProviderSlack:
		if st.SlackChannelID != "" {
			return st.SlackChannelID
		}
		return c.SlackChannelFallback
	}
	return ""
}

// Batch collects what one change should send. The main channel message is
// always refreshed; Thread and DMs are optional.
type Batch struct {
	Thread      string // threaded reply under the main message
	SkipChannel bool   // don't refresh the main message (alerts that change nothing visible)
	dms         []dmPayload
}

// ThreadReply sets the threaded reply text.
func (b *Batch) ThreadReply(text string) { b.Thread = text }

// DM queues a direct message to each user.
func (b *Batch) DM(userIDs []int64, text string) {
	for _, id := range userIDs {
		b.dms = append(b.dms, dmPayload{UserID: id, Text: text})
	}
}

type upsertPayload struct {
	FiringID int64  `json:"firing_id"`
	Thread   string `json:"thread,omitempty"`
}

type dmPayload struct {
	UserID   int64  `json:"user_id"`
	FiringID int64  `json:"firing_id,omitempty"`
	Text     string `json:"text"`
}

// Enqueue writes outbox rows for a change to firing firingID made by actorID.
// Call it inside the change's transaction. The actor isn't DMed about their
// own action. firingID 0 or b.SkipChannel skips the channel message.
func Enqueue(ctx context.Context, q *db.Queries, cfg Config, firingID, actorID int64, b *Batch) error {
	if firingID != 0 && !b.SkipChannel {
		st, err := q.GetSettings(ctx)
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(upsertPayload{FiringID: firingID, Thread: b.Thread})
		for _, p := range platforms {
			if !cfg.enabled(p) || (cfg.Channel(st, p) == "" && !cfg.DryRun) {
				continue
			}
			if err := q.EnqueueOutbox(ctx, db.EnqueueOutboxParams{Kind: db.OutboxKindChannelUpsert, Platform: p, Payload: payload}); err != nil {
				return err
			}
		}
	}

	var seen []int64
	for _, dm := range b.dms {
		if dm.UserID == actorID || slices.Contains(seen, dm.UserID) {
			continue
		}
		seen = append(seen, dm.UserID)
		accounts, err := q.GetUserAccounts(ctx, dm.UserID)
		if err != nil {
			return err
		}
		if len(accounts) == 0 {
			continue
		}
		// DMs go to the platform the user signed in with (their first account).
		p := accounts[0].Provider
		if !cfg.enabled(p) {
			continue
		}
		dm.FiringID = firingID
		payload, _ := json.Marshal(dm)
		if err := q.EnqueueOutbox(ctx, db.EnqueueOutboxParams{Kind: db.OutboxKindDm, Platform: p, Payload: payload}); err != nil {
			return err
		}
	}
	return nil
}
