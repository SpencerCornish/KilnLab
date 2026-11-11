# BookLab — Architecture Overview

Self-hosted hourly resource booking with post-use Stripe billing (community spaces: darkrooms, makerspaces, studios, etc.).

## Stack

- **Backend:** Go 1.26 (`cmd/server` + `internal/` packages: api, db, email, scheduler, stripe, webembed, config)
- **Frontend:** TypeScript + React 19 SPA, built with Vite, styled with Tailwind CSS 4; Stripe.js/React for payments, react-router, react-big-calendar, Tiptap (rich text), embedded into the Go binary via `embed.FS`
- **Database:** PostgreSQL (pgx driver, migrations on startup via golang-migrate, queries generated with **sqlc**; DB-level exclusion constraint prevents double-bookings)
- **HTTP:** chi router + chi cors

## External services

- **Stripe** (SetupIntent card-on-file + PaymentIntent charges after use)
- **SMTP** for transactional email (confirmation, reminder, cancellation, receipt)

## Shape

- Single Go server on `:8080` — serves the SPA and a REST API under `/api/*` (public booking endpoints + admin endpoints)
- Config entirely via env vars (envconfig, `.env` via godotenv)
- Deployment: Docker / docker-compose (dev and prod variants, Postgres + app); asdf-managed toolchain (Go, Node, pnpm)
- Tests: `tests/e2e` (Node-based e2e) + `.functest` functional tests; API contract documented in `api/openapi.yaml`

In short: Go monolith + embedded React SPA talking to Postgres, with Stripe and SMTP as the only outside dependencies.
