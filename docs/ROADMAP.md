# KilnLab Roadmap & Status

Source of truth for what's built, what's next, and what's blocked. The product spec is
[KILN_APP_SPEC.md](../KILN_APP_SPEC.md) (section refs like §6 point there). Decisions and open
questions live in [DECISIONS.md](DECISIONS.md).

**Update this file at the end of every work session.**

Legend: ✅ done & tested · 🟡 partial · ⬜ not started · 🚫 blocked on owner

_Last updated: 2026-11-11, end of M2 (code complete, awaiting owner check-in). M1 signed off by owner._

---

## Milestone summary

| Milestone | Status | Check-in with owner |
|---|:-:|---|
| M0 — Scaffold & auth | 🟡 code ✅, phone verification 🚫 | Done 2026-11-11 |
| M1 — Firings & requests | ✅ | Signed off 2026-11-11. Built on the defaults for open questions 1–5; multi-kiln added (D24) |
| M2 — Status lifecycle & kiln QR | ✅ code complete | **Check-in pending**; phone check of `/kiln` still to do |
| M3 — Notifications | ⬜ next | Needs Discord bot + Slack bot tokens and channel IDs from the owner to test for real |
| M4 — Jobs, public page, calendar, admin settings | ⬜ | |
| Stretch (§15) | ⬜ | Only after v1 and with owner's OK |

The spec says to check in with the owner at the end of each milestone. Don't roll into the next one without that.

---

## M0 — Scaffold & auth

| Item | Status | Notes |
|---|:-:|---|
| Project scaffold (Go + embedded React SPA, Makefile, Docker, CI) | ✅ | Mirrors BookLab with current dependency versions |
| DB migrations (full v1 schema in `001_initial`) | ✅ | All §8 tables exist, not just auth |
| Seed one kiln + default settings | ✅ | In the migration |
| Discord login + ArtLab guild check + kiln master role | ✅ | `internal/auth/discord.go`, unit tested against a fake Discord |
| Slack login + workspace check (+ guest rejection) | ✅ | `internal/auth/slack.go`; guest check needs `SLACK_BOT_TOKEN` |
| Admin bootstrap via `ADMIN_USER_IDS` | ✅ | Granted at login, never revoked at login |
| DB sessions (7 days), CSRF double-submit, logout | ✅ | Integration tested |
| Cross-browser OAuth return (mobile app bounce) | ✅ | Integration tested; see DECISIONS.md D6 |
| Dev login (`DEV_LOGIN=true`) | ✅ | Refused when `APP_BASE_URL` is https |
| CI workflow | 🟡 | Written, never run (no GitHub remote yet) |
| Create Discord app + Slack app, fill `.env` | 🚫 | Owner. Steps in README |
| Deploy to staging URL | 🚫 | Owner picks the host |
| **Login verified on phones via QR** (iOS Safari, Android Chrome, with the Discord/Slack apps installed) | 🚫 | Needs both of the above |

---

## M1 — Firings & requests

Acceptance (§14): propose, view, request space, approve/decline, withdraw; fullness math + override;
compatibility warnings; kiln-size footnote; min notice + cutoff; kiln master confirm, hold/release,
reschedule, cancel, close/reopen requests; unit tests for state machine, permissions, fullness.

| Item | Status | Notes |
|---|:-:|---|
| `internal/policy`: permission matrix (§4) + exhaustive tests | ✅ | `ProposerCanDecideRequests = false` (open question 1) |
| `internal/firing`: state machine (§6), pure + tests | ✅ | Confirm/Hold/Release/Cancel/Reschedule/Advance. `Advance` (loading…completed) is ready for M2 but has no API yet |
| Fullness math (§7) + tests | ✅ | `firing.ComputeFullness`, `WouldOverfill` |
| Compatibility check (§7) + tests | ✅ | Cone mismatch; glazed work into bisque (new `firing_requests.glazed`, migration 002). Warns, never blocks |
| Requests window, min notice + tests | ✅ | `RequestsOpen`, `RequestsCloseAt`, `ReopenCloseAt`, `CheckMinNotice` |
| sqlc queries (`queries/firings.sql`) | ✅ | Every change writes `firing_events` (`firing/events.go` lists types) |
| API: options, list, detail, create (min notice; KM bypass) | ✅ | `GET /api/options`, `GET/POST /api/firings`, `GET /api/firings/{id}` (detail includes per-user `can` flags) |
| API: request / edit / withdraw (before cutoff) | ✅ | `POST /firings/{id}/requests`, `PUT …/requests/{rid}`, `POST …/withdraw`. Warnings for compat + "may not fit" |
| API: approve / decline with note | ✅ | `POST …/requests/{rid}/decide`. Kiln masters only |
| API: confirm / hold / release (+ optional new start) / reschedule / cancel | ✅ | Reason required for hold + cancel |
| API: fullness override, close/reopen requests, assign loader, loader list | ✅ | `PUT …/fullness-override`, `PUT …/requests-window`, `PUT …/loader`, `GET /api/loaders` |
| Integration tests for the above | ✅ | `internal/api/firings_integration_test.go` (fake clock for cutoff) |
| UI: `/` home (my firings, upcoming, recently finished, propose) | ✅ | |
| UI: `/firings/new` propose form with kiln-size footnote | ✅ | Footnote shows a generic line until kiln dimensions are set (owner, open question 6) |
| UI: `/firings/:id` detail, fullness bar, requests, timeline, kiln master controls | ✅ | Bottom-sheet forms for reasons/dates. **Not yet checked on a real phone** |
| `make seed`: sample users + firings in various states | ✅ | Users match dev login names: Kim (KM), Sam (self-loader), Alice, Bob (BMS), Carol |
| Proposer editing their own entry | ⬜ | Not in spec; ask owner if wanted |
| Notifications for these events | ⬜ | M3: enqueue in the same transaction inside `mutateFiring` |

## M2 — Status lifecycle & kiln QR

| Item | Status | Notes |
|---|:-:|---|
| All status transitions (§6) with permissions | ✅ | `POST /api/firings/{id}/status`. Only kiln masters start the kiln (server-side, tested) |
| Timestamps + back-dating (kiln masters, ≤7 days, not before the last recorded step) | ✅ | `started_at`, `cooling_at`, `unloaded_at`, `ready_for_pickup_at`, `completed_at`; event `occurred_at` back-dated too |
| Correct a timestamp afterwards | ✅ | `PUT /api/firings/{id}/timestamps`; order enforced; est. unload follows a corrected start |
| START FIRING: program (settings list or free text) + duration → `estimated_unload_at` | ✅ | Programs come from `settings.programs` (`make seed` adds samples; editable in M4) |
| `/kiln` (§9B): relevant firings, auto-open single, pick list, "No active firing" | ✅ | `GET /api/kiln?k=<kiln id>`; QR per kiln (D24) |
| `/kiln/:id`: summary + only this user's next steps, confirm screen, success screen | ✅ | Events tagged `source = kiln_qr` |
| Same actions + time fixes on `/firings/:id` | ✅ | |
| `/admin/users`: self-loader toggle (KM), admin toggle (admin) | ✅ | Can't remove your own admin. `/admin` index links here |
| Integration tests | ✅ | `internal/api/lifecycle_integration_test.go` |
| **`/kiln` checked on a real phone** (incl. pick list) | ⬜ | Owner/dev to try via LAN URL; real QR needs staging |
| Ready-for-pickup DMs | ⬜ | M3 (the confirm screen already warns participants will be told) |
| Printable QR codes per kiln | ⬜ | Could live on `/admin/settings` (M4) |

## M3 — Notifications

| Item | Status | Notes |
|---|:-:|---|
| Outbox worker: retries with exponential backoff, ~8 attempts, then `failed` | ⬜ | Failures must never block state changes |
| Discord: embed main message, PATCH in place, thread replies, DMs | ⬜ | Handle "cannot DM" without endless retries |
| Slack: Block Kit main message, `chat.update`, `thread_ts` replies, DMs | ⬜ | Needs a bot token; webhooks can't edit |
| Render-hash dedupe (`last_rendered_hash`) | ⬜ | |
| DMs: approve/decline, reschedule/cancel/hold, ready for pickup | ⬜ | DM goes to the platform the user signed in with |

## M4 — Jobs, public page, calendar

| Item | Status | Notes |
|---|:-:|---|
| In-process scheduler (5-min tick) | ⬜ | Replaces `/api/cron/tick`, see D3 |
| Jobs: close requests, under-filled alert, unconfirmed reminder, missed start, missed unload | ⬜ | Idempotent via `*_sent_at`; tested |
| `/upcoming` public page (§9A) | ⬜ | Placeholder page exists |
| `/calendar.ics` | ⬜ | Go-served, not SPA |
| `/admin/settings` incl. kiln dimensions | ⬜ | |
| End-to-end acceptance scenario (§14) | ⬜ | Consider Hurl/Playwright like BookLab |

---

## Dev-experience backlog

| Item | Status |
|---|:-:|
| Single `make dev` (DB :5433, API :8080, Vite :5173 with `--host` for phones on LAN) | ✅ |
| Test DB auto-created by the dev container; `make test` runs integration tests | ✅ |
| Go hot reload: pin `air` as a Go tool + `.air.toml` | ⬜ |
| Default `.env.example` `APP_BASE_URL` to `http://localhost:5173` so real OAuth works through Vite | ⬜ |
| `make seed` sample data | ✅ |
| `make tunnel` (cloudflared) + docs for testing real OAuth on phones / Slack's https redirect requirement | ⬜ |
| Push to GitHub so CI runs | 🚫 owner |
