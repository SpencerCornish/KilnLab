// Package api is the HTTP layer: JSON API under /api, OAuth callbacks, and the embedded SPA.
package api

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/spencercornish/kilnlab/internal/auth"
	"github.com/spencercornish/kilnlab/internal/config"
	"github.com/spencercornish/kilnlab/internal/db"
	"github.com/spencercornish/kilnlab/internal/notify"
)

type Server struct {
	cfg       *config.Config
	pool      *pgxpool.Pool
	queries   *db.Queries
	providers map[string]auth.Provider
	webFS     fs.FS
	logger    *slog.Logger
	now       func() time.Time

	notifyCfg notify.Config
	notifier  *notify.Worker // optional; Kick()ed after changes so sends go out promptly

	trustedProxies []netip.Prefix
}

func New(cfg *config.Config, pool *pgxpool.Pool, providers map[string]auth.Provider, webFS fs.FS, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	logger := log.With("component", "api")
	trusted, err := cfg.TrustedProxyPrefixes()
	if err != nil {
		logger.Error("invalid trusted proxies, trusting none", "error", err)
	}
	return &Server{
		cfg: cfg, pool: pool, queries: db.New(pool), providers: providers, webFS: webFS,
		logger: logger, now: time.Now, trustedProxies: trusted, notifyCfg: NotifyConfig(cfg),
	}
}

// NotifyConfig derives the notification settings from the app config.
func NotifyConfig(cfg *config.Config) notify.Config {
	return notify.Config{
		BaseURL: cfg.AppBaseURL, DiscordBotToken: cfg.DiscordBotToken, SlackBotToken: cfg.SlackBotToken,
		DiscordChannelFallback: cfg.DiscordChannelIDFallback, SlackChannelFallback: cfg.SlackChannelIDFallback,
		DryRun: cfg.NotificationsDryRun,
	}
}

// SetNotifier lets the server wake the outbox worker after each change.
func (s *Server) SetNotifier(w *notify.Worker) { s.notifier = w }

func (s *Server) Addr() string { return fmt.Sprintf(":%d", s.cfg.Port) }

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()

	r.Use(s.securityHeaders)
	r.Use(middleware.Recoverer)
	r.Use(s.requestLogger)
	r.Use(middleware.GetHead) // uptime monitors often probe with HEAD
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.cfg.AllowedCORSOrigins(),
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
	}))

	r.Route("/api", func(r chi.Router) {
		r.Use(s.loadSession)
		r.Use(s.csrfProtect)

		r.Get("/health", s.handleHealth)
		r.Get("/public/upcoming", s.handleUpcoming)

		// Auth. start/callback are top-level browser navigations, not fetches.
		r.Get("/auth/providers", s.handleAuthProviders)
		r.Get("/auth/{provider}/start", s.handleAuthStart)
		r.Get("/auth/{provider}/callback", s.handleAuthCallback)
		r.Post("/auth/logout", s.handleLogout)
		if s.cfg.DevLogin {
			r.Post("/auth/dev", s.handleDevLogin)
		}

		r.Group(func(r chi.Router) {
			r.Use(s.requireUser)
			r.Get("/me", s.handleMe)
			r.Get("/options", s.handleOptions)
			r.Get("/kiln", s.handleKiln)
			r.Get("/admin/users", s.handleListUsers)
			r.Patch("/admin/users/{id}", s.handleUpdateUser)
			r.Get("/admin/notifications", s.handleListOutbox)
			r.Get("/admin/settings", s.handleGetSettings)
			r.Put("/admin/settings", s.handleUpdateSettings)
			r.Post("/admin/kilns", s.handleCreateKiln)
			r.Put("/admin/kilns/{id}", s.handleUpdateKiln)

			r.Get("/firings", s.handleListFirings)
			r.Post("/firings", s.handleCreateFiring)
			r.Route("/firings/{id}", func(r chi.Router) {
				r.Get("/", s.handleGetFiring)

				r.Post("/requests", s.handleCreateRequest)
				r.Put("/requests/{rid}", s.handleUpdateRequest)
				r.Post("/requests/{rid}/withdraw", s.handleWithdrawRequest)
				r.Post("/requests/{rid}/decide", s.handleDecideRequest)

				// Kiln master controls.
				r.Post("/confirm", s.handleConfirm)
				r.Post("/hold", s.handleHold)
				r.Post("/release", s.handleRelease)
				r.Post("/reschedule", s.handleReschedule)
				r.Post("/cancel", s.handleCancel)
				r.Put("/fullness-override", s.handleFullnessOverride)
				r.Put("/requests-window", s.handleRequestsWindow)

				// Lifecycle: loading … completed, and fixing times afterwards.
				r.Post("/status", s.handleAdvance)
				r.Put("/timestamps", s.handleCorrectTimestamp)
			})
		})
	})

	r.Get("/calendar.ics", s.handleCalendar)
	r.Get("/*", s.handleSPA)
	return r
}

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		s.logger.Log(r.Context(), httpLogLevel(r.URL.Path, status), "http_request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
			"client_ip", s.clientIP(r),
		)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.pool.Ping(r.Context()); err != nil {
		s.writeError(w, r, http.StatusServiceUnavailable, "database unavailable", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
