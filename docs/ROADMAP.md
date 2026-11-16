# KilnLab Roadmap & Status

Source of truth for what's built, what's next, and what's blocked. The product spec is
[KILN_APP_SPEC.md](../KILN_APP_SPEC.md) (section refs like §6 point there). Decisions and open
questions live in [DECISIONS.md](DECISIONS.md).

**Update this file at the end of every work session.**

Legend: ✅ done & tested · 🟡 partial · ⬜ not started · 🚫 blocked on owner

_Last updated: 2026-11-16, firing ownership + profiles (branch `feature/firing-ownership-profiles`). The pre-prod schema was reset to one clean baseline; see the note below. M4/v1 remains code-complete; real OAuth apps, bots, staging and phone checks are still owner-blocked._

_Post-M4 owner testing changes: unloaded, ready for pickup and completed merged into one `completed` step (migration 004, [DECISIONS](DECISIONS.md) #13). Also: firings are booked by day, not start time (migration 005, [DECISIONS](DECISIONS.md) #14), and the loader role is gone: anyone signed in can say they've loaded or unloaded (migration 006, #15). The owner has more changes to come._

_2026-11-14 UI refresh: the **Kiln Control** design system is applied across the SPA (D47): plain `kc-*` classes + tokens in `web/src/styles/`, self-hosted fonts, 56px tap targets, and segmented capacity meters. Layouts checked at 320px and 390px wide; still worth a pass on a real phone._

_2026-11-14 post-v1 additions (branch `feature/km-alerts-and-blackouts`): new firing requests DM every kiln master (D49); a day-of reminder fires once at an admin-set hour (default 8am studio time) on each firing's day (D50, migration 007); admins can black out date ranges per kiln for maintenance, a hard block for everyone with no override, gating new proposals and reschedules only (D51, migration 008). Unit + integration tested; `make test`, `go vet` and `tsc` green._

_2026-11-14 UI declutter (branch `ui-clutter`, not yet merged): firing page consolidated (one Actions section; program/times + timeline folded into a collapsed History; hold/cancel reasons as header captions), home button bank trimmed to Propose + Admin, "Recently finished" capped at 3, ShareInput is presets-only (no slider), fullness meters show one number (room-left on the public page), plate heads flattened, admin index gated to admins (was an empty page for kiln masters), settings groups collapsed into disclosures. Home is one soonest-first list (no "mine" vs "upcoming" split; firings you're in get a highlighted plate), "Recently finished" is a collapsed disclosure, and firing cards list who's in under the meter, each with its share (`ListFiringParticipants`). Cards no longer print "by &lt;proposer&gt;"; the name list carries it._

_2026-11-16 firing ownership + profiles (branch `feature/firing-ownership-profiles`): the pre-prod schema was reset to a single clean baseline. Firing profiles are per kiln (name, type, cone, rough firing/cooldown times) and chosen by the proposer, with an **Other** custom option. Confirm is replaced by **Take this firing** (`managed_by_user_id`); routine alerts go only to the owner, or all KMs while unclaimed, with missed-start/unload escalation. Cooling is automatic at `started_at + firing_minutes`; "ready to unload" is derived from the profile's cooldown; anyone can unload early. Loading/loaded and the redundant unloaded/ready timestamps are gone. **No data migration** — `make db-reset` is required. See [DECISIONS](DECISIONS.md) D52–D57._


---

## Milestone summary

| Milestone | Status | Check-in with owner |
|---|:-:|---|
| M0 — Scaffold & auth | 🟡 code ✅, phone verification 🚫 | Done 2026-11-11 |
| M1 — Firings & requests | ✅ | Signed off 2026-11-11. Built on the defaults for open questions 1–5; multi-kiln added (D24) |
| M2 — Status lifecycle & kiln QR | ✅ | Signed off 2026-11-11. Phone check of `/kiln` still to do |
| M3 — Notifications | ✅ | Signed off 2026-11-11. Real bots still need owner tokens + channel IDs |
| M4 — Jobs, public page, calendar, admin settings | ✅ | Signed off 2026-11-11 |
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
| API: fullness override, close/reopen requests | ✅ | `PUT …/fullness-override`, `PUT …/requests-window` (loader assignment removed, #15) |
| Integration tests for the above | ✅ | `internal/api/firings_integration_test.go` (fake clock for cutoff) |
| UI: `/` home (my firings, upcoming, recently finished, propose) | ✅ | |
| UI: `/firings/new` propose form with kiln-size footnote | ✅ | Footnote shows a generic line until kiln dimensions are set (owner, open question 6) |
| UI: `/firings/:id` detail, fullness bar, requests, timeline, kiln master controls | ✅ | Bottom-sheet forms for reasons/dates. **Not yet checked on a real phone** |
| `make seed`: sample users + firings in various states | ✅ | Users match dev login names: Kim (KM), Sam, Alice, Bob (BMS), Carol |
| Proposer editing their own entry | ⬜ | Not in spec; ask owner if wanted |
| Notifications for these events | ⬜ | M3: enqueue in the same transaction inside `mutateFiring` |

## M2 — Status lifecycle & kiln QR

| Item | Status | Notes |
|---|:-:|---|
| All status transitions (§6) with permissions | ✅ | `POST /api/firings/{id}/status`. Only kiln masters start the kiln (server-side, tested) |
| Timestamps + back-dating (kiln masters, ≤7 days, not before the last recorded step) | ✅ | `started_at`, `firing_ends_at`, `unloadable_at`, `cooling_at`, `unloaded_at`; event `occurred_at` back-dated too |
| Correct a timestamp afterwards | ✅ | `PUT /api/firings/{id}/timestamps`; order enforced; the two estimates follow a corrected start |
| START FIRING: one tap using the firing profile; automatic cooling and unloadable time | ✅ | D52–D55; profile supplies firing + cooldown minutes. No program/duration typed |
| `/kiln` (§9B): relevant firings, auto-open single, pick list, "No active firing" | ✅ | `GET /api/kiln?k=<kiln id>`; QR per kiln (D24) |
| `/kiln/:id`: summary + only this user's next steps, confirm screen, success screen | ✅ | Events tagged `source = kiln_qr` |
| Same actions + time fixes on `/firings/:id` | ✅ | |
| `/admin/users`: admin toggle (admin only; self-loader removed, #15) | ✅ | Can't remove your own admin. `/admin` index links here |
| Integration tests | ✅ | `internal/api/lifecycle_integration_test.go` |
| **`/kiln` checked on a real phone** (incl. pick list) | ⬜ | Owner/dev to try via LAN URL; real QR needs staging |
| Ready-for-pickup DMs | ✅ | Completed sends DMs to the proposer + approved requesters (M3) |
| Printable QR codes per kiln | ⬜ | Could live on `/admin/settings` (M4) |

## M3 — Notifications

| Item | Status | Notes |
|---|:-:|---|
| Outbox enqueued in the same transaction as every change | ✅ | `notify.Enqueue` from `mutateFiring` / create. Failures never block a change |
| Outbox worker: backoff 30s→1h, 8 attempts, then `failed`; permanent errors fail at once | ✅ | `internal/notify/worker.go`; woken right after each change, else polls every 10s |
| Discord: embed main message, PATCH in place, thread created on first reply, DMs | ✅ | "Cannot DM" (403) fails permanently, no endless retries. Deleted message → reposted |
| Slack: Block Kit main message, `chat.update`, `thread_ts` replies, DMs to user id | ✅ | |
| Render-hash dedupe (`last_rendered_hash`) | ✅ | Unchanged cards aren't re-edited |
| Thread replies: started, ready for pickup, cancelled, rescheduled | ✅ | Open question 11 default |
| DMs: approved/declined (with note), hold/reschedule/cancel (all participants), ready (approved + proposer) | ✅ | To the platform the user signed in with; the actor is never DMed about their own action |
| Ownership-aware DMs | ✅ | Requests/cutoff/day-of go to the owner, or all KMs while unclaimed. Missed start/unload escalates to all KMs + admins (D53) |
| `NOTIFICATIONS_DRY_RUN` for local dev (logs instead of sending, no tokens needed) | ✅ | On in `.env.example` |
| Admin notification log on `/admin` (status, attempts, last error) | ✅ | `GET /api/admin/notifications` |
| Tests | ✅ | `internal/notify/*_test.go` (fake APIs), `internal/api/notify_integration_test.go` |
| **Real Discord bot + Slack bot posting to real channels** | 🚫 | Owner: bot tokens, invite bots, channel IDs (README) |
| Kiln-master alert DMs (under-filled, needs owner, day-of, missed start/unload, escalation) | ✅ | M4 jobs; `notify.Enqueue` + the responsible-recipient helper |

## M4 — Jobs, public page, calendar

| Item | Status | Notes |
|---|:-:|---|
| In-process scheduler (5-min tick, plus once at startup) | ✅ | `internal/jobs` (`Runner`), started from `cmd/server`. One transaction per firing; one failure doesn't stop the others |
| Jobs: auto-cooling, close requests, under-filled alert, needs-owner reminder, day-of, missed start/unload, escalation | ✅ | Pure `jobs.Due` + unit tests; idempotent via `*_sent_at` + `requests_close_handled_at`. D33–D37, D52–D55 |
| Re-arming after reschedule / reopen | ✅ | Integration tested (`internal/api/jobs_integration_test.go`, incl. §14 step 9) |
| Timeline shows system actions ("· automatic") | ✅ | `alert_sent` events, source `system` |
| `/upcoming` public page (§9A) | ✅ | `GET /api/public/upcoming`, no names/notes (tested). "Request space" → login → request form opens (`/firings/:id?request=1`) |
| `/calendar.ics` | ✅ | `internal/ical` (escaping, folding, tested). D39. Linked from `/upcoming` (webcal + copy link) |
| `/admin/settings`: all settings, per-kiln firing profiles, channel IDs, bot status | ✅ | `GET/PUT /api/admin/settings` + `/api/admin/profiles`, validated (tested) |
| Kilns: add / edit dimensions / deactivate | ✅ | `POST /api/admin/kilns`, `PUT /api/admin/kilns/{id}`; at least one stays active |
| Printable QR codes (wheel + per kiln) | ✅ | `/admin/qr` (D41) |
| **Phone check of `/upcoming`, `/admin/settings` and printed QR codes** | ⬜ | Needs staging for real QR scans |
| End-to-end acceptance scenario (§14) as one automated test | ⬜ | Covered piecewise by the integration tests. A Hurl/Playwright run could follow after staging |

---

## Security review (2026-11-14, after deploying to kiln.bozemanartlab.com)

| Item | Status | Notes |
|---|:-:|---|
| Go 1.27.4 (was 1.26.3, 21 reachable stdlib vulnerabilities) | ✅ | `go.mod`, `.tool-versions`, `Dockerfile`. `govulncheck` clean. **Redeploy with `--pull`** |
| Delete expired sessions / OAuth states; rate limit sign-in | ✅ | D45 |
| Hourly membership + Kiln Master re-check; admin "Sign out" | ✅ | D43, D44. Needs the bot tokens in production |
| Escape member text in Slack/Discord DMs and thread replies | ✅ | D46 |
| CI runs on pushes to `master` | ✅ | Was `main` |
| Login CSRF when the callback has no binding cookie | ⬜ | Accepted mobile trade-off; a "Continue as …?" step would close it |
| `__Host-` cookie prefix | ⬜ | Stops sibling subdomains setting our cookies |
| HTTP read/write/idle timeouts, CAA record, digest-pinned images | ⬜ | Hardening |

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
