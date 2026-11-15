package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// SendError classifies a failed send so the worker knows whether to retry.
type SendError struct {
	Msg        string
	Permanent  bool          // don't retry (bad channel, user blocks DMs, …)
	NotFound   bool          // the message we wanted to edit is gone: post a new one
	RetryAfter time.Duration // rate limited: wait at least this long
}

func (e *SendError) Error() string { return e.Msg }

func asSendError(err error) *SendError {
	var se *SendError
	if errors.As(err, &se) {
		return se
	}
	return nil
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

// ── Discord ─────────────────────────────────────────────────────────────────

// Discord is a minimal bot client for the REST API.
type Discord struct {
	token   string
	baseURL string
}

func NewDiscord(token, baseURL string) *Discord {
	if baseURL == "" {
		baseURL = "https://discord.com/api/v10"
	}
	return &Discord{token: token, baseURL: strings.TrimRight(baseURL, "/")}
}

// Discord JSON error codes we treat specially.
const (
	discordUnknownChannel = 10003
	discordUnknownMessage = 10008
	discordCannotDM       = 50007
)

func (d *Discord) do(ctx context.Context, method, path string, body, out any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, method, d.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+d.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "DiscordBot (https://github.com/spencercornish/kilnlab, 1.0)")
	resp, err := httpClient.Do(req)
	if err != nil {
		return &SendError{Msg: "discord: " + err.Error()}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 300 {
		if out != nil {
			return json.Unmarshal(raw, out)
		}
		return nil
	}
	var apiErr struct {
		Code       int     `json:"code"`
		Message    string  `json:"message"`
		RetryAfter float64 `json:"retry_after"`
	}
	_ = json.Unmarshal(raw, &apiErr)
	se := &SendError{Msg: fmt.Sprintf("discord %s %s: %d %s (code %d)", method, path, resp.StatusCode, apiErr.Message, apiErr.Code)}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		se.RetryAfter = time.Duration(apiErr.RetryAfter * float64(time.Second))
	case apiErr.Code == discordUnknownMessage:
		se.NotFound = true
	case resp.StatusCode >= 500:
		// retry
	default:
		se.Permanent = true // bad token, missing access, unknown channel, cannot DM user, …
	}
	return se
}

type discordEmbed struct {
	Title       string `json:"title"`
	URL         string `json:"url,omitempty"`
	Description string `json:"description"`
	Color       int    `json:"color"`
}

type discordMessage struct {
	Content         string           `json:"content,omitempty"`
	Embeds          []discordEmbed   `json:"embeds,omitempty"`
	AllowedMentions map[string][]any `json:"allowed_mentions"`
}

func noMentions() map[string][]any { return map[string][]any{"parse": {}} }

// discordEscape stops member-written text (titles, reasons, names) from
// rendering as a masked link like [Sign in again](https://evil.example).
// Bare URLs still show as themselves, which is honest.
func discordEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "[", `\[`, "]", `\]`).Replace(s)
}

func discordCardMessage(c Card) discordMessage {
	title := discordEscape(c.Title)
	if c.Cancelled {
		title = "~~" + title + "~~"
	}
	lines := make([]string, len(c.Lines))
	for i, l := range c.Lines {
		lines[i] = discordEscape(l)
	}
	return discordMessage{
		Embeds: []discordEmbed{{
			Title: truncate(title, 256), URL: c.URL, Color: c.Color,
			Description: truncate(strings.Join(lines, "\n")+"\n\n[View / request space →]("+c.URL+")", 4096),
		}},
		AllowedMentions: noMentions(),
	}
}

// PostCard creates the main message and returns its id.
func (d *Discord) PostCard(ctx context.Context, channel string, c Card) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	err := d.do(ctx, http.MethodPost, "/channels/"+channel+"/messages", discordCardMessage(c), &out)
	return out.ID, err
}

// EditCard edits the main message in place.
func (d *Discord) EditCard(ctx context.Context, channel, messageID string, c Card) error {
	return d.do(ctx, http.MethodPatch, "/channels/"+channel+"/messages/"+messageID, discordCardMessage(c), nil)
}

// StartThread creates a thread from the main message and returns its channel id.
func (d *Discord) StartThread(ctx context.Context, channel, messageID, name string) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	err := d.do(ctx, http.MethodPost, "/channels/"+channel+"/messages/"+messageID+"/threads",
		map[string]any{"name": truncate(name, 100), "auto_archive_duration": 10080}, &out)
	return out.ID, err
}

// PostText sends a plain message (thread replies, DMs). The text includes
// member-written parts, so it is escaped.
func (d *Discord) PostText(ctx context.Context, channel, text string) error {
	return d.do(ctx, http.MethodPost, "/channels/"+channel+"/messages",
		discordMessage{Content: truncate(discordEscape(text), 2000), AllowedMentions: noMentions()}, nil)
}

// DM opens a DM channel with the user and sends text. Users who don't share
// the guild with the bot or block DMs give a permanent error.
func (d *Discord) DM(ctx context.Context, userID, text string) error {
	var ch struct {
		ID string `json:"id"`
	}
	if err := d.do(ctx, http.MethodPost, "/users/@me/channels", map[string]string{"recipient_id": userID}, &ch); err != nil {
		return err
	}
	return d.PostText(ctx, ch.ID, text)
}

// ── Slack ───────────────────────────────────────────────────────────────────

// Slack is a minimal bot client for chat.postMessage / chat.update.
type Slack struct {
	token   string
	baseURL string
}

func NewSlack(token, baseURL string) *Slack {
	if baseURL == "" {
		baseURL = "https://slack.com/api"
	}
	return &Slack{token: token, baseURL: strings.TrimRight(baseURL, "/")}
}

var slackPermanent = []string{
	"channel_not_found", "not_in_channel", "is_archived", "invalid_auth", "not_authed", "account_inactive",
	"token_revoked", "user_not_found", "cannot_dm_bot", "missing_scope", "msg_too_long", "invalid_blocks",
	"no_text", "restricted_action", "team_access_not_granted",
}

type slackResponse struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error"`
	TS      string `json:"ts"`
	Channel string `json:"channel"`
}

func (s *Slack) call(ctx context.Context, method string, body any) (*slackResponse, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/"+method, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, &SendError{Msg: "slack: " + err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		secs, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		return nil, &SendError{Msg: "slack " + method + ": rate limited", RetryAfter: time.Duration(secs) * time.Second}
	}
	if resp.StatusCode >= 300 {
		return nil, &SendError{Msg: fmt.Sprintf("slack %s: HTTP %d", method, resp.StatusCode)}
	}
	var out slackResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, &SendError{Msg: "slack " + method + ": bad response: " + err.Error()}
	}
	if !out.OK {
		se := &SendError{Msg: "slack " + method + ": " + out.Error}
		switch {
		case out.Error == "message_not_found":
			se.NotFound = true
		case out.Error == "ratelimited":
			se.RetryAfter = time.Minute
		case slices.Contains(slackPermanent, out.Error):
			se.Permanent = true
		}
		return nil, se
	}
	return &out, nil
}

// mrkdwnEscape escapes the three characters Slack treats as control characters.
func mrkdwnEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func slackBlocks(c Card) []any {
	title := mrkdwnEscape(c.Title)
	if c.Cancelled {
		title = "~" + title + "~"
	}
	lines := make([]string, len(c.Lines))
	for i, l := range c.Lines {
		lines[i] = mrkdwnEscape(l)
	}
	return []any{
		map[string]any{"type": "section", "text": map[string]string{
			"type": "mrkdwn", "text": truncate(fmt.Sprintf("*<%s|%s>*\n%s", c.URL, title, strings.Join(lines, "\n")), 3000),
		}},
		map[string]any{"type": "actions", "elements": []any{map[string]any{
			"type": "button", "text": map[string]string{"type": "plain_text", "text": "View / request space"}, "url": c.URL,
		}}},
	}
}

// PostCard creates the main message and returns (channel id, ts).
func (s *Slack) PostCard(ctx context.Context, channel string, c Card) (string, string, error) {
	out, err := s.call(ctx, "chat.postMessage", map[string]any{
		"channel": channel, "text": mrkdwnEscape(c.Text()), "blocks": slackBlocks(c), "unfurl_links": false,
	})
	if err != nil {
		return "", "", err
	}
	return out.Channel, out.TS, nil
}

// UpdateCard edits the main message in place.
func (s *Slack) UpdateCard(ctx context.Context, channel, ts string, c Card) error {
	_, err := s.call(ctx, "chat.update", map[string]any{"channel": channel, "ts": ts, "text": mrkdwnEscape(c.Text()), "blocks": slackBlocks(c)})
	return err
}

// PostText sends a plain message; threadTS replies in a thread; channel may be a user id (DM).
// The text includes member-written parts, so it is escaped: otherwise a title
// like <https://evil.example|Sign in again> would render as a disguised link.
func (s *Slack) PostText(ctx context.Context, channel, text, threadTS string) error {
	body := map[string]any{"channel": channel, "text": truncate(mrkdwnEscape(text), 3000), "unfurl_links": false}
	if threadTS != "" {
		body["thread_ts"] = threadTS
	}
	_, err := s.call(ctx, "chat.postMessage", body)
	return err
}
