package api

import (
	"net/http"
	"strings"
)

// contentSecurityPolicy: the SPA's own assets plus Discord/Slack avatar images.
var contentSecurityPolicy = strings.Join([]string{
	"default-src 'self'",
	"script-src 'self'",
	"style-src 'self' 'unsafe-inline'",
	"img-src 'self' data: https://cdn.discordapp.com https://*.slack-edge.com https://secure.gravatar.com",
	"connect-src 'self'",
	"object-src 'none'",
	"base-uri 'self'",
	"form-action 'self'",
	"frame-ancestors 'none'",
}, "; ")

// securityHeaders sets browser hardening headers. HSTS only when APP_BASE_URL is https.
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	hsts := s.cfg.SecureCookies()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if hsts {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}
