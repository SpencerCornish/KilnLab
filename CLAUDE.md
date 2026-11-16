# KilnLab — notes for coding agents

Read before starting work:
1. [docs/ROADMAP.md](docs/ROADMAP.md): milestone status, what's next, what's blocked. **Update it at the end of each session.**
2. [docs/DECISIONS.md](docs/DECISIONS.md): decisions made, plus the spec's open questions and the defaults in use.
3. [KILN_APP_SPEC.md](KILN_APP_SPEC.md): the product spec (§ references point here).
4. [README.md](README.md): setup, commands, Discord/Slack app config.

`ARCHITECTURE.md` describes **BookLab** (the owner's other app, at `~/repos/booklab`), kept as reference only.

## Working rules
- Work milestone by milestone (M0–M4). Check in with the owner at the end of each one.
- Keep the core logic pure and unit tested: role predicates in `internal/policy`; state machine, requests window, fullness, compatibility and profile timings in `internal/firing`; job conditions in `internal/jobs` (`Due`).
- Permissions are enforced on the server. Only kiln masters can take a firing, start the kiln, or make kiln-master decisions. Anyone signed in can unload.
- One kiln master owns a firing; routine alerts go to that owner, or to every KM while it is unclaimed. Missed start/unload escalates to all KMs + admins.
- Notification failures must never block a state change; all sends go through `notification_outbox`.
- Use current dependency versions (look them up); don't copy BookLab's pins.
- Mobile-first UI with large tap targets. Use the Kiln Control design system classes (`.kc-key`, `.kc-plate`, `.kc-input`, …) from `web/src/styles/kiln-control.css`; tokens live in `web/src/styles/tokens.css`. The reference spec is in `kiln-control/`.

## Workflow
- **The app is pre-production and uses a single clean schema baseline.** Change `internal/db/migrations/001_initial.up.sql` (and the down file), then `make sqlc`. Do not add incremental migrations yet; run `make db-reset` to rebuild the dev DB. Ask the owner before assuming data can be discarded.
- Queries go in `internal/db/queries/*.sql`, then `make sqlc`.
- `make dev`: DB on :5433, API on :8080, Vite on :5173. Use the dev login form (`DEV_LOGIN=true`) to sign in as any role.
- `make seed`: sample users, per-kiln profiles and firings (names match the dev login form).
- `make test`: unit + Postgres integration tests (`TEST_DATABASE_URL`, `kilnlab_test` DB).
- `make lint`: go vet, gofmt, tsc.
- SPA routes must be listed in both `web/src/App.tsx` and `isSPARoute` in `internal/api/spa.go`.
