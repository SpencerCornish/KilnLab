// Package config loads KilnLab configuration from environment variables.
package config

import (
	"fmt"
	"log"
	"net/netip"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`
	Port        int    `envconfig:"PORT" default:"8080"`

	// AppBaseURL is the public URL with scheme and no trailing slash. OAuth
	// redirect URIs and links in Discord/Slack messages are built from it.
	AppBaseURL string `envconfig:"APP_BASE_URL" default:"http://localhost:8080"`

	// Discord (ArtLab)
	DiscordClientID          string `envconfig:"DISCORD_CLIENT_ID"`
	DiscordClientSecret      string `envconfig:"DISCORD_CLIENT_SECRET"`
	DiscordBotToken          string `envconfig:"DISCORD_BOT_TOKEN"`
	ArtLabGuildID            string `envconfig:"ARTLAB_GUILD_ID"`
	DiscordKilnMasterRoleID  string `envconfig:"DISCORD_KILN_MASTER_ROLE_ID"`
	DiscordChannelIDFallback string `envconfig:"DISCORD_CHANNEL_ID"`

	// Slack (BMS)
	SlackClientID          string `envconfig:"SLACK_CLIENT_ID"`
	SlackClientSecret      string `envconfig:"SLACK_CLIENT_SECRET"`
	SlackBotToken          string `envconfig:"SLACK_BOT_TOKEN"`
	SlackTeamID            string `envconfig:"SLACK_TEAM_ID"`
	SlackChannelIDFallback string `envconfig:"SLACK_CHANNEL_ID"`
	// SlackAllowGuests lets multi- and single-channel guests sign in. Checked
	// with users.info, so it only applies when SLACK_BOT_TOKEN is set.
	SlackAllowGuests bool `envconfig:"SLACK_ALLOW_GUESTS" default:"false"`

	// AdminUserIDs bootstraps admins: comma-separated provider user IDs
	// (Discord snowflakes or Slack member IDs, optionally prefixed "discord:"/"slack:").
	AdminUserIDs string `envconfig:"ADMIN_USER_IDS"`

	// DevLogin enables /api/auth/dev, which signs in as a fake user with
	// chosen roles. For local development and functional tests only; refused
	// when APP_BASE_URL is https.
	DevLogin bool `envconfig:"DEV_LOGIN" default:"false"`

	CORSAllowedOrigins string `envconfig:"CORS_ALLOWED_ORIGINS" default:""`
	TrustedProxies     string `envconfig:"TRUSTED_PROXIES" default:"127.0.0.0/8,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,fc00::/7"`
}

// Load reads the environment and exits on invalid configuration.
func Load() *Config {
	cfg := &Config{}
	if err := envconfig.Process("", cfg); err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config: %v", err)
	}
	return cfg
}

// Validate checks cross-field rules envconfig cannot express.
func (c *Config) Validate() error {
	c.AppBaseURL = strings.TrimRight(strings.TrimSpace(c.AppBaseURL), "/")
	if !strings.HasPrefix(c.AppBaseURL, "http://") && !strings.HasPrefix(c.AppBaseURL, "https://") {
		return fmt.Errorf("APP_BASE_URL must start with http:// or https://")
	}
	if c.DevLogin && c.SecureCookies() {
		return fmt.Errorf("DEV_LOGIN must not be enabled when APP_BASE_URL is https")
	}
	if c.DiscordEnabled() && (c.ArtLabGuildID == "" || c.DiscordKilnMasterRoleID == "") {
		return fmt.Errorf("ARTLAB_GUILD_ID and DISCORD_KILN_MASTER_ROLE_ID are required when Discord login is configured")
	}
	if c.SlackEnabled() && c.SlackTeamID == "" {
		return fmt.Errorf("SLACK_TEAM_ID is required when Slack login is configured")
	}
	if !c.DiscordEnabled() && !c.SlackEnabled() && !c.DevLogin {
		return fmt.Errorf("no login method configured: set Discord and/or Slack OAuth credentials (or DEV_LOGIN=true locally)")
	}
	if _, err := c.TrustedProxyPrefixes(); err != nil {
		return err
	}
	return nil
}

func (c *Config) DiscordEnabled() bool { return c.DiscordClientID != "" && c.DiscordClientSecret != "" }
func (c *Config) SlackEnabled() bool   { return c.SlackClientID != "" && c.SlackClientSecret != "" }

// SecureCookies reports whether cookies should carry the Secure attribute.
// Plain-http localhost development would otherwise drop them in some browsers.
func (c *Config) SecureCookies() bool { return strings.HasPrefix(c.AppBaseURL, "https://") }

// IsBootstrapAdmin reports whether a provider user ID is in ADMIN_USER_IDS.
func (c *Config) IsBootstrapAdmin(provider, providerUserID string) bool {
	for _, part := range strings.Split(c.AdminUserIDs, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if p, id, ok := strings.Cut(part, ":"); ok {
			if p == provider && id == providerUserID {
				return true
			}
			continue
		}
		if part == providerUserID {
			return true
		}
	}
	return false
}

// SessionCookieName is the name of the login session cookie.
const SessionCookieName = "kilnlab_session"

// SessionDuration is how long a login lasts. Roles are re-derived at each login.
const SessionDuration = 7 * 24 * time.Hour

// TrustedProxyPrefixes parses TrustedProxies. Bare IPs are treated as single-address ranges.
func (c *Config) TrustedProxyPrefixes() ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, part := range strings.Split(c.TrustedProxies, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(part); err == nil {
			out = append(out, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(part)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXIES: invalid IP or CIDR %q", part)
		}
		addr = addr.Unmap()
		out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return out, nil
}

// AllowedCORSOrigins returns origins for the CORS middleware; defaults to AppBaseURL.
func (c *Config) AllowedCORSOrigins() []string {
	var out []string
	for _, part := range strings.Split(c.CORSAllowedOrigins, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return []string{c.AppBaseURL}
	}
	return out
}
