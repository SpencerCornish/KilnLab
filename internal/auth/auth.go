// Package auth talks to Discord and Slack to turn an OAuth authorization code
// into a verified community member identity.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Identity is a verified member as reported by their community's provider.
type Identity struct {
	Provider       string // "discord" | "slack"
	ProviderUserID string
	TeamOrGuildID  string
	Community      string // "artlab" | "bms"
	DisplayName    string
	AvatarURL      string
	IsKilnMaster   bool
}

// RejectedError means the provider authenticated the person but they are not
// allowed in (wrong guild/workspace, guest account). Code is passed to the
// login page, which maps it to a fixed message (never echo free text from a URL).
type RejectedError struct{ Code string }

const (
	RejectNotInGuild       = "not_in_guild"
	RejectWrongWorkspace   = "wrong_workspace"
	RejectSlackGuest       = "slack_guest"
	RejectSlackDeactivated = "slack_deactivated"
)

func (e *RejectedError) Error() string { return "login rejected: " + e.Code }

// Provider is implemented by Discord and Slack.
type Provider interface {
	Name() string
	AuthCodeURL(state string) string
	Exchange(ctx context.Context, code string) (*Identity, error)
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// getJSON performs an authenticated GET and decodes a JSON body. It returns the
// HTTP status so callers can distinguish "not a member" (404) from outages.
func getJSON(ctx context.Context, url, authHeader string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return resp.StatusCode, fmt.Errorf("GET %s: decode: %w", url, err)
	}
	return resp.StatusCode, nil
}
