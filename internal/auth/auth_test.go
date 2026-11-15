package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// fakeProvider serves a token endpoint plus whatever API handlers a test adds.
func fakeProvider(t *testing.T, tokenPath string, api map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(tokenPath, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("code") != "good-code" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"user-token","token_type":"Bearer","ok":true}`))
	})
	for p, h := range api {
		mux.HandleFunc(p, h)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func jsonHandler(t *testing.T, wantAuth string, status int, body any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != wantAuth {
			t.Errorf("Authorization = %q, want %q", got, wantAuth)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}
}

func rejectCode(err error) string {
	var rej *RejectedError
	if errors.As(err, &rej) {
		return rej.Code
	}
	return ""
}

func newTestDiscord(base string) *Discord {
	return NewDiscord(DiscordConfig{
		ClientID: "cid", ClientSecret: "secret", RedirectURL: "https://app/cb",
		GuildID: "guild1", KilnMasterRoleID: "role-km", BaseURL: base,
	})
}

func TestDiscordAuthCodeURL(t *testing.T) {
	d := newTestDiscord("")
	u, err := url.Parse(d.AuthCodeURL("st"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Host != "discord.com" || q.Get("state") != "st" || q.Get("scope") != "identify guilds.members.read" {
		t.Fatalf("unexpected auth URL %s", u)
	}
}

func TestDiscordExchange(t *testing.T) {
	nick, global, av := "Cody K", "Cody", "abc"
	memberPath := "/api/v10/users/@me/guilds/guild1/member"

	tests := []struct {
		name       string
		status     int
		member     any
		wantReject string
		wantKM     bool
		wantName   string
	}{
		{
			name:   "kiln master with nickname",
			status: 200,
			member: discordMember{User: discordUser{ID: "u1", Username: "cody", GlobalName: &global, Avatar: &av}, Nick: &nick, Roles: []string{"other", "role-km"}},
			wantKM: true, wantName: "Cody K",
		},
		{
			name:     "member without role falls back to global name",
			status:   200,
			member:   discordMember{User: discordUser{ID: "u2", Username: "emily", GlobalName: &global}, Roles: []string{"other"}},
			wantName: "Cody",
		},
		{
			name:     "username when no names set",
			status:   200,
			member:   discordMember{User: discordUser{ID: "u3", Username: "annabelle"}},
			wantName: "annabelle",
		},
		{name: "not in guild", status: 404, member: map[string]any{"message": "Unknown Guild"}, wantReject: RejectNotInGuild},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := fakeProvider(t, "/api/oauth2/token", map[string]http.HandlerFunc{
				memberPath: jsonHandler(t, "Bearer user-token", tt.status, tt.member),
			})
			id, err := newTestDiscord(srv.URL).Exchange(context.Background(), "good-code")
			if tt.wantReject != "" {
				if rejectCode(err) != tt.wantReject {
					t.Fatalf("err = %v, want reject %s", err, tt.wantReject)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if id.IsKilnMaster != tt.wantKM || id.DisplayName != tt.wantName || id.Community != "artlab" || id.Provider != "discord" {
				t.Fatalf("identity = %+v", id)
			}
		})
	}
}

func TestDiscordExchangeOutageIsNotRejection(t *testing.T) {
	srv := fakeProvider(t, "/api/oauth2/token", map[string]http.HandlerFunc{
		"/api/v10/users/@me/guilds/guild1/member": jsonHandler(t, "Bearer user-token", 502, map[string]any{}),
	})
	_, err := newTestDiscord(srv.URL).Exchange(context.Background(), "good-code")
	if err == nil || rejectCode(err) != "" {
		t.Fatalf("want plain error for outage, got %v", err)
	}
}

func TestDiscordBadCode(t *testing.T) {
	srv := fakeProvider(t, "/api/oauth2/token", nil)
	if _, err := newTestDiscord(srv.URL).Exchange(context.Background(), "bad"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSlackAuthCodeURLPreselectsTeam(t *testing.T) {
	s := NewSlack(SlackConfig{ClientID: "cid", ClientSecret: "s", RedirectURL: "https://app/cb", TeamID: "T1"})
	u, _ := url.Parse(s.AuthCodeURL("st"))
	if u.Query().Get("team") != "T1" || !strings.Contains(u.Query().Get("scope"), "openid") {
		t.Fatalf("unexpected auth URL %s", u)
	}
}

func TestSlackExchange(t *testing.T) {
	userInfo := func(team string) map[string]any {
		return map[string]any{
			"ok": true, "sub": "U123", "name": "Real Name",
			"https://slack.com/team_id":        team,
			"https://slack.com/user_image_192": "https://avatars.slack-edge.com/x.png",
		}
	}
	usersInfo := func(restricted, ultra bool, display string) map[string]any {
		return map[string]any{"ok": true, "user": map[string]any{
			"is_restricted": restricted, "is_ultra_restricted": ultra,
			"profile": map[string]any{"display_name": display},
		}}
	}

	tests := []struct {
		name        string
		team        string
		botToken    string
		allowGuests bool
		users       map[string]any
		wantReject  string
		wantName    string
	}{
		{name: "member without bot token", team: "T1", wantName: "Real Name"},
		{name: "wrong workspace", team: "T999", wantReject: RejectWrongWorkspace},
		{name: "full member with display name", team: "T1", botToken: "xoxb", users: usersInfo(false, false, "annie"), wantName: "annie"},
		{name: "multi-channel guest rejected", team: "T1", botToken: "xoxb", users: usersInfo(true, false, ""), wantReject: RejectSlackGuest},
		{name: "single-channel guest rejected", team: "T1", botToken: "xoxb", users: usersInfo(false, true, ""), wantReject: RejectSlackGuest},
		{name: "guest allowed when configured", team: "T1", botToken: "xoxb", allowGuests: true, users: usersInfo(true, false, ""), wantName: "Real Name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := map[string]http.HandlerFunc{
				"/api/openid.connect.userInfo": jsonHandler(t, "Bearer user-token", 200, userInfo(tt.team)),
			}
			if tt.users != nil {
				api["/api/users.info"] = func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Query().Get("user") != "U123" {
						t.Errorf("users.info user = %q", r.URL.Query().Get("user"))
					}
					jsonHandler(t, "Bearer "+tt.botToken, 200, tt.users)(w, r)
				}
			}
			srv := fakeProvider(t, "/api/openid.connect.token", api)
			s := NewSlack(SlackConfig{
				ClientID: "cid", ClientSecret: "s", RedirectURL: "https://app/cb", TeamID: "T1",
				BotToken: tt.botToken, AllowGuests: tt.allowGuests, BaseURL: srv.URL,
			})
			id, err := s.Exchange(context.Background(), "good-code")
			if tt.wantReject != "" {
				if rejectCode(err) != tt.wantReject {
					t.Fatalf("err = %v, want reject %s", err, tt.wantReject)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if id.DisplayName != tt.wantName || id.Community != "bms" || id.ProviderUserID != "U123" || id.IsKilnMaster {
				t.Fatalf("identity = %+v", id)
			}
		})
	}
}

func TestDiscordCheckMember(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    any
		want    Membership
		wantErr bool
	}{
		{"member", 200, map[string]any{"roles": []string{"other"}}, Membership{Member: true}, false},
		{"kiln master", 200, map[string]any{"roles": []string{"role-km"}}, Membership{Member: true, KilnMaster: true}, false},
		{"left the guild", 404, map[string]any{"code": 10007}, Membership{}, false},
		{"deleted account", 404, map[string]any{"code": 10013}, Membership{}, false},
		// The bot was removed from the guild: not the member's fault, so no verdict.
		{"unknown guild", 404, map[string]any{"code": 10004}, Membership{}, true},
		{"missing access", 403, map[string]any{"code": 50001}, Membership{}, true},
		{"outage", 502, "bad gateway", Membership{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := fakeProvider(t, "/api/oauth2/token", map[string]http.HandlerFunc{
				"/api/v10/guilds/guild1/members/42": jsonHandler(t, "Bot bot-token", tt.status, tt.body),
			})
			d := NewDiscord(DiscordConfig{GuildID: "guild1", KilnMasterRoleID: "role-km", BotToken: "bot-token", BaseURL: srv.URL})
			got, err := d.CheckMember(context.Background(), "42")
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("CheckMember = %+v, %v; want %+v, error %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestSlackCheckMember(t *testing.T) {
	user := func(fields map[string]any) map[string]any { return map[string]any{"ok": true, "user": fields} }
	tests := []struct {
		name        string
		status      int
		body        any
		allowGuests bool
		want        Membership
		wantErr     bool
	}{
		{"member", 200, user(map[string]any{}), false, Membership{Member: true}, false},
		{"deactivated", 200, user(map[string]any{"deleted": true}), false, Membership{}, false},
		{"guest", 200, user(map[string]any{"is_restricted": true}), false, Membership{}, false},
		{"guest allowed", 200, user(map[string]any{"is_restricted": true}), true, Membership{Member: true}, false},
		{"unknown user", 200, map[string]any{"ok": false, "error": "user_not_found"}, false, Membership{}, false},
		{"bad token", 200, map[string]any{"ok": false, "error": "invalid_auth"}, false, Membership{}, true},
		{"outage", 503, "unavailable", false, Membership{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := fakeProvider(t, "/api/openid.connect.token", map[string]http.HandlerFunc{
				"/api/users.info": jsonHandler(t, "Bearer bot-token", tt.status, tt.body),
			})
			s := NewSlack(SlackConfig{TeamID: "T1", BotToken: "bot-token", AllowGuests: tt.allowGuests, BaseURL: srv.URL})
			got, err := s.CheckMember(context.Background(), "U1")
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("CheckMember = %+v, %v; want %+v, error %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}
