# KilnLab Roadmap & Status

Source of truth for what's built, what's next, and what's blocked. The product spec is
[KILN_APP_SPEC.md](../KILN_APP_SPEC.md) (section refs like §6 point there). Decisions and open
questions live in [DECISIONS.md](DECISIONS.md).

**Update this file at the end of every work session.**

Legend: ✅ done & tested · 🟡 partial · ⬜ not started · 🚫 blocked on owner

_Last updated: 2026-11-11, end of M0 (code complete)._

---

## Milestone summary

| Milestone | Status | Check-in with owner |
|---|:-:|---|
| M0 — Scaffold & auth | 🟡 code ✅, phone verification 🚫 | Done 2026-11-11 |
| M1 — Firings & requests | ⬜ next | Waiting on answers to open questions 1–5 (DECISIONS.md), or go with defaults |
| M2 — Status lifecycle & kiln QR | ⬜ | |
| M3 — Notifications | ⬜ | |
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
| `internal/policy`: permission matrix (§4) as pure functions + exhaustive tests | 🟡 | `Roles` type exists; per-action checks still to do |
| `internal/firing` (or similar): state machine (§6) as pure logic + tests | ⬜ | Includes skip rules, release → proposed/confirmed via `was_confirmed`, requests window |
| Fullness math (§7) + tests | ⬜ | proposer share + approved requests, cap 100 for display, override wins |
| Compatibility check (§7) | ⬜ | cone mismatch; glazed work into bisque. Warn, never block |
| sqlc queries for firings / requests / events | ⬜ | Every transition writes `firing_events` |
| API: create firing (min notice; kiln master bypass) | ⬜ | `requests_close_at = scheduled_start − cutoff` |
| API: list/get firing, my firings | ⬜ | |
| API: request space / edit / withdraw (before cutoff) | ⬜ | Warn if over 100% |
| API: approve / decline with note | ⬜ | Who may approve: open question 1 |
| API: confirm / hold (reason) / release / reschedule / cancel (reason) | ⬜ | |
| API: fullness override, close/reopen requests, assign loader | ⬜ | |
| UI: `/` home (upcoming + my firings + propose) | ⬜ | Currently shows identity + roles only |
| UI: `/firings/new` propose form with kiln-size footnote | ⬜ | Needs kiln dimensions (owner) |
| UI: `/firings/:id` detail, fullness bar, requests, timeline, kiln master controls | ⬜ | |
| `make seed`: sample users + firings in various states | ⬜ | Dev-friendliness item |

## M2 — Status lifecycle & kiln QR

| Item | Status | Notes |
|---|:-:|---|
| All status transitions (§6) with permissions + back-dating | ⬜ | Only kiln masters can start the kiln, enforced on the server |
| Timestamps: started/cooling/unloaded/ready/completed, `estimated_unload_at` | ⬜ | §12: back-dating must be accurate |
| `/kiln` flow (§9B): relevant-firing detection, pick list, "no active firing" | ⬜ | Placeholder page exists |
| Confirm screens per action; START FIRING with program + duration → est. unload | ⬜ | `source = kiln_qr` on events |
| Same actions on `/firings/:id` (fixing missed scans from home) | ⬜ | |
| `/admin/users`: toggle self-loader (kiln master), toggle admin (admin) | ⬜ | |

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
| `make seed` sample data | ⬜ (with M1) |
| `make tunnel` (cloudflared) + docs for testing real OAuth on phones / Slack's https redirect requirement | ⬜ |
| Push to GitHub so CI runs | 🚫 owner |
