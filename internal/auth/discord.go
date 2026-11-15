package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"golang.org/x/oauth2"
)

const discordDefaultBase = "https://discord.com"

// Discord signs in ArtLab members. Membership and the kiln master role come
// from the user's member record in the ArtLab guild.
type Discord struct {
	oauth    *oauth2.Config
	apiBase  string // e.g. https://discord.com/api/v10
	cdnBase  string
	guildID  string
	kmRoleID string
	botToken string
}

type DiscordConfig struct {
	ClientID, ClientSecret, RedirectURL string
	GuildID, KilnMasterRoleID           string
	// BotToken enables CheckMember (re-checking membership between logins).
	BotToken string
	// BaseURL overrides https://discord.com (tests).
	BaseURL string
}

func NewDiscord(c DiscordConfig) *Discord {
	base := c.BaseURL
	if base == "" {
		base = discordDefaultBase
	}
	return &Discord{
		oauth: &oauth2.Config{
			ClientID:     c.ClientID,
			ClientSecret: c.ClientSecret,
			RedirectURL:  c.RedirectURL,
			Scopes:       []string{"identify", "guilds.members.read"},
			Endpoint: oauth2.Endpoint{
				AuthURL:   base + "/oauth2/authorize",
				TokenURL:  base + "/api/oauth2/token",
				AuthStyle: oauth2.AuthStyleInParams,
			},
		},
		apiBase:  base + "/api/v10",
		cdnBase:  "https://cdn.discordapp.com",
		guildID:  c.GuildID,
		kmRoleID: c.KilnMasterRoleID,
		botToken: c.BotToken,
	}
}

func (d *Discord) Name() string { return "discord" }

func (d *Discord) AuthCodeURL(state string) string {
	// prompt=none skips the consent screen for people who already authorized.
	return d.oauth.AuthCodeURL(state, oauth2.SetAuthURLParam("prompt", "none"))
}

type discordUser struct {
	ID         string  `json:"id"`
	Username   string  `json:"username"`
	GlobalName *string `json:"global_name"`
	Avatar     *string `json:"avatar"`
}

type discordMember struct {
	User   discordUser `json:"user"`
	Nick   *string     `json:"nick"`
	Avatar *string     `json:"avatar"`
	Roles  []string    `json:"roles"`
}

func (d *Discord) Exchange(ctx context.Context, code string) (*Identity, error) {
	tok, err := d.oauth.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("discord token exchange: %w", err)
	}
	var m discordMember
	status, err := getJSON(ctx, d.apiBase+"/users/@me/guilds/"+d.guildID+"/member", "Bearer "+tok.AccessToken, &m)
	if err != nil {
		if status == http.StatusNotFound || status == http.StatusForbidden {
			return nil, &RejectedError{Code: RejectNotInGuild}
		}
		return nil, fmt.Errorf("discord guild member: %w", err)
	}
	if m.User.ID == "" {
		return nil, fmt.Errorf("discord guild member: response missing user")
	}
	return &Identity{
		Provider:       "discord",
		ProviderUserID: m.User.ID,
		TeamOrGuildID:  d.guildID,
		Community:      "artlab",
		DisplayName:    discordDisplayName(m),
		AvatarURL:      d.avatarURL(m),
		IsKilnMaster:   slices.Contains(m.Roles, d.kmRoleID),
	}, nil
}

// CanCheckMembers reports whether a bot token is configured.
func (d *Discord) CanCheckMembers() bool { return d.botToken != "" }

// Discord JSON error codes that mean "this person isn't in the guild".
const (
	discordUnknownMember = 10007
	discordUnknownUser   = 10013
)

// CheckMember looks the user up in the ArtLab guild with the bot token. Only
// Discord saying the member or user is unknown counts as "left". Any other
// failure (outage, bot removed from the guild, bad token) is an error, so a
// misconfigured bot never signs everyone out.
func (d *Discord) CheckMember(ctx context.Context, userID string) (Membership, error) {
	u := d.apiBase + "/guilds/" + d.guildID + "/members/" + url.PathEscape(userID)
	status, body, err := getBody(ctx, u, "Bot "+d.botToken)
	if err != nil {
		return Membership{}, fmt.Errorf("discord guild member: %w", err)
	}
	if status == http.StatusOK {
		var m discordMember
		if err := json.Unmarshal(body, &m); err != nil {
			return Membership{}, fmt.Errorf("discord guild member: decode: %w", err)
		}
		return Membership{Member: true, KilnMaster: slices.Contains(m.Roles, d.kmRoleID)}, nil
	}
	var apiErr struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(body, &apiErr)
	if status == http.StatusNotFound && (apiErr.Code == discordUnknownMember || apiErr.Code == discordUnknownUser) {
		return Membership{}, nil
	}
	return Membership{}, fmt.Errorf("discord guild member: status %d (code %d)", status, apiErr.Code)
}

// discordDisplayName prefers the guild nickname, then the global display name, then the username.
func discordDisplayName(m discordMember) string {
	for _, s := range []*string{m.Nick, m.User.GlobalName} {
		if s != nil && strings.TrimSpace(*s) != "" {
			return strings.TrimSpace(*s)
		}
	}
	return m.User.Username
}

func (d *Discord) avatarURL(m discordMember) string {
	if m.Avatar != nil && *m.Avatar != "" {
		return fmt.Sprintf("%s/guilds/%s/users/%s/avatars/%s.png?size=128", d.cdnBase, d.guildID, m.User.ID, *m.Avatar)
	}
	if m.User.Avatar != nil && *m.User.Avatar != "" {
		return fmt.Sprintf("%s/avatars/%s/%s.png?size=128", d.cdnBase, m.User.ID, *m.User.Avatar)
	}
	return ""
}
