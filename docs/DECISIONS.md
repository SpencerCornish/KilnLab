# Decisions & Open Questions

Record of choices made while building KilnLab, and the spec's **[CONFIRM]** questions with the
default currently in use. When the owner answers a question, move it to Decisions with the date.

---

## Decisions

| # | Date | Decision | Why |
|---|---|---|---|
| D1 | 2026-11-11 | **Stack matches BookLab** (`~/repos/booklab`): Go 1.26 + chi + pgx + golang-migrate + sqlc, React SPA (Vite/Tailwind) embedded via `embed.FS`, Docker compose, Makefile | Owner request. Spec §0.1 says to match existing apps |
| D2 | 2026-11-11 | **Use current dependency versions, not BookLab's pins** (React 19.3, react-router 8, Vite 8, TS 7, Tailwind 4.3, Postgres 18, chi 5.5, pgx 5.13, oauth2 0.38, sqlc 1.31.1) | Owner: "no need to copy this directly, I want more up to date dependencies" |
| D3 | 2026-11-11 | **Scheduler is an in-process ticker goroutine**, not an external cron calling `/api/cron/tick` (spec §3) | Single binary like BookLab; nothing external to configure. Jobs stay idempotent via `*_sent_at` |
| D4 | 2026-11-11 | **SPA instead of server-rendered pages** (spec §0.4) | Follows from D1. `/calendar.ics` will still be served directly by Go |
| D5 | 2026-11-11 | **Real sqlc codegen** (`make sqlc`, CI runs `sqlc diff`), run via `go run …@v1.31.1` instead of a go.mod tool dependency | BookLab's "sqlc" layer is actually hand-written. A tool dependency would pull sqlc's large module graph into go.mod |
| D6 | 2026-11-11 | **OAuth state stored in the DB** (`oauth_states`), plus a per-browser binding cookie that is checked *when present* | Phones may come back from the Discord/Slack app into a different browser. Trade-off: weaker protection against login CSRF, acceptable here |
| D7 | 2026-11-11 | **Login errors are passed as fixed codes** (`/login?error=not_in_guild`), mapped to messages in the SPA | Never render free text from a URL |
| D8 | 2026-11-11 | **Session cookie holds a random token; DB stores its SHA-256** | A DB leak doesn't yield usable sessions |
| D9 | 2026-11-11 | **Admin is granted at login from `ADMIN_USER_IDS` but never revoked at login** | Admins granted in the app stay admins |
| D10 | 2026-11-11 | **Kiln master lives on `accounts.is_kiln_master`**, refreshed every Discord login; a user is a kiln master if any of their accounts is | Ready for account linking later |
| D11 | 2026-11-11 | **Admins can do everything kiln masters can** (`Roles.CanManageFirings`) | Matches the spec §4 matrix |
| D12 | 2026-11-11 | **Dev Postgres on host port 5433** | BookLab's dev DB uses 5432; both can run at once |
| D13 | 2026-11-11 | **`DEV_LOGIN`**: fake sign-in with any role for local dev; server refuses to start with it on when `APP_BASE_URL` is https | Develop and test without real Discord/Slack apps |
| D14 | 2026-11-11 | Added `settings.underfilled_threshold_pct` (default 50) and `firings.was_confirmed` (for release → confirmed) and `notification_messages.thread_id` (Discord thread) beyond the §8 table list | Needed by §6/§10/§11 behaviour |
| D15 | 2026-11-11 | Kiln dimensions are `double precision`, not `numeric` | Plain floats in Go instead of `pgtype.Numeric` |

---

## Open questions (spec §17), current defaults

**Questions 1–5 change code logic. Ask before or during M1.** The rest are just values in settings.

| # | Question | Default in use | Affects |
|---|---|---|---|
| 1 | Can a firing's proposer approve/decline requests, or only kiln masters? | **Kiln masters only**; proposer gets notified | M1 policy |
| 2 | Allow Slack guests? | **No** (`SLACK_ALLOW_GUESTS=false`; only enforced when `SLACK_BOT_TOKEN` is set) | M0, done |
| 3 | One "on duty" kiln master per firing, or alert all kiln masters? | **All kiln masters** | M3/M4 |
| 4 | Show participant names on the public page? | **Count only** | M4 |
| 5 | `completed` automatic N days after ready-for-pickup, or manual? | **Manual** | M2/M4 |
| 6 | Kiln interior dimensions, shelf count, "one shelf ≈ X%" | Unset (kiln seeded as "Studio kiln" with null dimensions) | M1 footnote |
| 7 | Minimum notice / request cutoff | 48h / 12h before start | settings |
| 8 | Under-filled alert threshold | 50% | settings |
| 9 | Firing types, cones, controller programs + typical durations | types `bisque, glaze, other`; cones `022 018 010 06 04 5 6 10`; programs empty | settings |
| 10 | Discord and Slack channel IDs for notifications | Unset | M3 |
| 11 | Threaded replies for which events? | Started, ready for pickup, cancelled, rescheduled | M3 |
| 12 | Hosting / staging URL | Unknown (prod compose assumes a VPS + Caddy/nginx) | M0 phone check |
