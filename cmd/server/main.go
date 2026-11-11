package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // studio timezone without relying on the container having zoneinfo

	"github.com/joho/godotenv"

	"github.com/spencercornish/kilnlab/internal/api"
	"github.com/spencercornish/kilnlab/internal/auth"
	"github.com/spencercornish/kilnlab/internal/config"
	"github.com/spencercornish/kilnlab/internal/db"
	"github.com/spencercornish/kilnlab/internal/jobs"
	"github.com/spencercornish/kilnlab/internal/notify"
	"github.com/spencercornish/kilnlab/internal/webembed"
)

func main() {
	_ = godotenv.Load()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	providers := map[string]auth.Provider{}
	if cfg.DiscordEnabled() {
		providers["discord"] = auth.NewDiscord(auth.DiscordConfig{
			ClientID:         cfg.DiscordClientID,
			ClientSecret:     cfg.DiscordClientSecret,
			RedirectURL:      cfg.AppBaseURL + "/api/auth/discord/callback",
			GuildID:          cfg.ArtLabGuildID,
			KilnMasterRoleID: cfg.DiscordKilnMasterRoleID,
		})
	}
	if cfg.SlackEnabled() {
		if cfg.SlackBotToken == "" {
			slog.Warn("SLACK_BOT_TOKEN not set: Slack guest accounts cannot be detected at login")
		}
		providers["slack"] = auth.NewSlack(auth.SlackConfig{
			ClientID:     cfg.SlackClientID,
			ClientSecret: cfg.SlackClientSecret,
			RedirectURL:  cfg.AppBaseURL + "/api/auth/slack/callback",
			TeamID:       cfg.SlackTeamID,
			BotToken:     cfg.SlackBotToken,
			AllowGuests:  cfg.SlackAllowGuests,
		})
	}
	if cfg.DevLogin {
		slog.Warn("DEV_LOGIN enabled: anyone can sign in as any user. Never enable this in production.")
	}

	// Prefer the embedded SPA; fall back to disk for `go run` after `make build-web`.
	var webFS fs.FS = webembed.FS
	if webFS == nil {
		const distDir = "internal/webembed/dist"
		if _, err := os.Stat(distDir + "/index.html"); err == nil {
			webFS = os.DirFS(distDir)
		} else {
			slog.Warn("spa dist missing; API only", "path", distDir)
		}
	}

	srv := api.New(cfg, pool, providers, webFS, logger)

	// Notification outbox worker (Discord/Slack). Sends never block requests.
	notifyCfg := api.NotifyConfig(cfg)
	switch {
	case cfg.NotificationsDryRun:
		slog.Warn("NOTIFICATIONS_DRY_RUN enabled: Discord/Slack messages are logged, not sent")
	case cfg.DiscordBotToken == "" && cfg.SlackBotToken == "":
		slog.Warn("no DISCORD_BOT_TOKEN or SLACK_BOT_TOKEN: notifications are off")
	}
	worker := notify.NewWorker(pool, notifyCfg, logger, notify.WorkerOptions{})
	srv.SetNotifier(worker)
	go worker.Run(ctx)

	// Scheduled jobs (spec §10): request cutoffs, kiln-master alerts, missed-scan reminders.
	go jobs.NewRunner(pool, notifyCfg, worker, logger).Run(ctx)

	httpSrv := &http.Server{
		Addr:              srv.Addr(),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	slog.Info("kilnlab listening", "addr", httpSrv.Addr, "base_url", cfg.AppBaseURL)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("http server failed", "error", err)
		os.Exit(1)
	}
}
