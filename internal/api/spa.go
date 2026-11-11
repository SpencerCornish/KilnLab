package api

import (
	"log/slog"
	"net/http"
	"path"
	"strings"
)

// spaRoutes are paths handled by the React router (see web/src/App.tsx).
var spaExact = map[string]bool{"/": true, "/login": true, "/upcoming": true, "/kiln": true}
var spaPrefixes = []string{"/firings/", "/admin/", "/kiln/"}

func isSPARoute(rawPath string) bool {
	p := path.Clean(rawPath)
	if spaExact[p] || p == "/admin" || p == "/firings" {
		return true
	}
	for _, prefix := range spaPrefixes {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if s.webFS == nil {
		http.Error(w, "frontend not built (run make build-web)", http.StatusServiceUnavailable)
		return
	}
	rel := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if rel != "" && rel != "index.html" {
		if f, err := s.webFS.Open(rel); err == nil {
			st, statErr := f.Stat()
			f.Close()
			if statErr == nil && !st.IsDir() {
				if strings.HasPrefix(rel, "assets/") {
					// Vite assets are content-hashed.
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				http.ServeFileFS(w, r, s.webFS, rel)
				return
			}
		}
	}
	if isSPARoute(r.URL.Path) {
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, s.webFS, "index.html")
		return
	}
	http.NotFound(w, r)
}

func httpLogLevel(p string, status int) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case strings.HasPrefix(p, "/assets/") && status < 400:
		return slog.LevelDebug
	case status == http.StatusNotFound && !strings.HasPrefix(p, "/api/"):
		return slog.LevelDebug // scanner noise
	case status >= 400:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}
