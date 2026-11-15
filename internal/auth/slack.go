package auth

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/oauth2"
)

const slackDefaultBase = "https://slack.com"

// Slack signs in BMS members via Sign in with Slack (OpenID Connect).
type Slack struct {
	oauth       *oauth2.Config
	apiBase     string
	teamID      string
	botToken    string
	allowGuests bool
}

type SlackConfig struct {
	ClientID, ClientSecret, RedirectURL string
	TeamID                              string
	// BotToken (users:read) enables the guest check and better display names.
	BotToken    string
	AllowGuests bool
	// BaseURL overrides https://slack.com (tests).
	BaseURL string
}

func NewSlack(c SlackConfig) *Slack {
	base := c.BaseURL
	if base == "" {
		base = slackDefaultBase
	}
	return &Slack{
		oauth: &oauth2.Config{
			ClientID:     c.ClientID,
			ClientSecret: c.ClientSecret,
			RedirectURL:  c.RedirectURL,
			Scopes:       []string{"openid", "profile", "email"},
			Endpoint: oauth2.Endpoint{
				AuthURL:   base + "/openid/connect/authorize",
				TokenURL:  base + "/api/openid.connect.token",
				AuthStyle: oauth2.AuthStyleInParams,
			},
		},
		apiBase:     base + "/api",
		teamID:      c.TeamID,
		botToken:    c.BotToken,
		allowGuests: c.AllowGuests,
	}
}

func (s *Slack) Name() string { return "slack" }

func (s *Slack) AuthCodeURL(state string) string {
	// team= preselects the BMS workspace; membership is still verified after login.
	return s.oauth.AuthCodeURL(state, oauth2.SetAuthURLParam("team", s.teamID))
}

type slackUserInfo struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error"`
	Sub     string `json:"sub"`
	TeamID  string `json:"https://slack.com/team_id"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
	Image   string `json:"https://slack.com/user_image_192"`
}

type slackUsersInfo struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
	User  struct {
		Deleted           bool `json:"deleted"`
		IsRestricted      bool `json:"is_restricted"`
		IsUltraRestricted bool `json:"is_ultra_restricted"`
		Profile           struct {
			DisplayName string `json:"display_name"`
			RealName    string `json:"real_name"`
		} `json:"profile"`
	} `json:"user"`
}

func (s *Slack) Exchange(ctx context.Context, code string) (*Identity, error) {
	tok, err := s.oauth.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("slack token exchange: %w", err)
	}
	// userInfo is fetched server-to-server with the token we just exchanged
	// using our client secret, so its claims are trustworthy without separately
	// verifying the id_token signature.
	var ui slackUserInfo
	if _, err := getJSON(ctx, s.apiBase+"/openid.connect.userInfo", "Bearer "+tok.AccessToken, &ui); err != nil {
		return nil, fmt.Errorf("slack userInfo: %w", err)
	}
	if !ui.OK || ui.Sub == "" {
		return nil, fmt.Errorf("slack userInfo: %s", ui.Error)
	}
	if ui.TeamID != s.teamID {
		return nil, &RejectedError{Code: RejectWrongWorkspace}
	}

	id := &Identity{
		Provider:       "slack",
		ProviderUserID: ui.Sub,
		TeamOrGuildID:  ui.TeamID,
		Community:      "bms",
		DisplayName:    strings.TrimSpace(ui.Name),
		AvatarURL:      firstNonEmpty(ui.Image, ui.Picture),
	}

	if s.botToken != "" {
		var info slackUsersInfo
		if _, err := getJSON(ctx, s.apiBase+"/users.info?user="+url.QueryEscape(ui.Sub), "Bearer "+s.botToken, &info); err != nil || !info.OK {
			return nil, fmt.Errorf("slack users.info: %v %s", err, info.Error)
		}
		if info.User.Deleted {
			return nil, &RejectedError{Code: RejectSlackDeactivated}
		}
		if !s.allowGuests && (info.User.IsRestricted || info.User.IsUltraRestricted) {
			return nil, &RejectedError{Code: RejectSlackGuest}
		}
		if dn := strings.TrimSpace(info.User.Profile.DisplayName); dn != "" {
			id.DisplayName = dn
		}
	}
	if id.DisplayName == "" {
		id.DisplayName = "BMS member"
	}
	return id, nil
}

// CanCheckMembers reports whether a bot token is configured.
func (s *Slack) CanCheckMembers() bool { return s.botToken != "" }

// CheckMember applies the login rules (not deactivated, not a guest unless
// allowed) to an existing account, using users.info with the bot token.
func (s *Slack) CheckMember(ctx context.Context, userID string) (Membership, error) {
	var info slackUsersInfo
	if _, err := getJSON(ctx, s.apiBase+"/users.info?user="+url.QueryEscape(userID), "Bearer "+s.botToken, &info); err != nil {
		return Membership{}, fmt.Errorf("slack users.info: %w", err)
	}
	if !info.OK {
		if info.Error == "user_not_found" {
			return Membership{}, nil
		}
		return Membership{}, fmt.Errorf("slack users.info: %s", info.Error)
	}
	if info.User.Deleted || (!s.allowGuests && (info.User.IsRestricted || info.User.IsUltraRestricted)) {
		return Membership{}, nil
	}
	return Membership{Member: true}, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
