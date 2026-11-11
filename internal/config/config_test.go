package config

import "testing"

func TestIsBootstrapAdmin(t *testing.T) {
	c := &Config{AdminUserIDs: " 1234 , slack:U9, discord:55"}
	cases := []struct {
		provider, id string
		want         bool
	}{
		{"discord", "1234", true},
		{"slack", "1234", true}, // bare IDs match any provider
		{"slack", "U9", true},
		{"discord", "U9", false},
		{"discord", "55", true},
		{"slack", "55", false},
		{"discord", "", false},
	}
	for _, tc := range cases {
		if got := c.IsBootstrapAdmin(tc.provider, tc.id); got != tc.want {
			t.Errorf("IsBootstrapAdmin(%q,%q) = %v, want %v", tc.provider, tc.id, got, tc.want)
		}
	}
}

func TestValidate(t *testing.T) {
	base := func() *Config {
		return &Config{AppBaseURL: "https://kiln.example.org/", DiscordClientID: "a", DiscordClientSecret: "b", ArtLabGuildID: "g", DiscordKilnMasterRoleID: "r"}
	}
	c := base()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.AppBaseURL != "https://kiln.example.org" {
		t.Errorf("trailing slash not trimmed: %q", c.AppBaseURL)
	}

	c = base()
	c.DevLogin = true
	if c.Validate() == nil {
		t.Error("DEV_LOGIN on https must be rejected")
	}

	c = base()
	c.ArtLabGuildID = ""
	if c.Validate() == nil {
		t.Error("Discord without guild ID must be rejected")
	}

	c = &Config{AppBaseURL: "http://localhost:8080"}
	if c.Validate() == nil {
		t.Error("no login method must be rejected")
	}
	c.DevLogin = true
	if err := c.Validate(); err != nil {
		t.Errorf("dev login on http should be allowed: %v", err)
	}
}
