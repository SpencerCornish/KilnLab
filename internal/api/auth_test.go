package api

import "testing"

func TestSafeReturnTo(t *testing.T) {
	cases := map[string]string{
		"":                        "/",
		"/kiln":                   "/kiln",
		"/firings/12?x=1":         "/firings/12?x=1",
		"//evil.example":          "/",
		"/\\evil.example":         "/",
		"https://evil.example/":   "/",
		"kiln":                    "/",
		"/api/auth/discord/start": "/",
		"/ok\r\nSet-Cookie: x":    "/",
	}
	for in, want := range cases {
		if got := safeReturnTo(in); got != want {
			t.Errorf("safeReturnTo(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsSPARoute(t *testing.T) {
	for _, p := range []string{"/", "/login", "/kiln", "/upcoming", "/firings/new", "/firings/3", "/admin/settings"} {
		if !isSPARoute(p) {
			t.Errorf("%s should be an SPA route", p)
		}
	}
	for _, p := range []string{"/wp-admin", "/.env", "/calendar.ics", "/api/me"} {
		if isSPARoute(p) {
			t.Errorf("%s should not be an SPA route", p)
		}
	}
}
