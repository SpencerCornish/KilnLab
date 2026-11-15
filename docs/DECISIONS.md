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
| D16 | 2026-11-11 | **M1 built on the defaults for open questions 1–5** | Owner said "begin the next milestone". Q1 is one constant: `policy.ProposerCanDecideRequests` |
| D17 | 2026-11-11 | State machine details the spec left open: **hold** only from proposed/confirmed; **forward skips** only from confirmed onwards (proposed must be confirmed first); **self-loaders** may move to loading/loaded/unloaded, can skip within those, never past the kiln start (superseded by #15: no loader role) | Simplest reading of §6. All in `internal/firing/statemachine.go` |
| D18 | 2026-11-11 | **One live (pending/approved) request per user per firing** (partial unique index). The proposer can't request in their own firing | Their share is the proposer entry |
| D19 | 2026-11-11 | **Editing an approved request sends it back to pending** | The kiln master approved specific work and share |
| D20 | 2026-11-11 | **Reopening requests after the automatic cutoff extends the window to the start time** | Otherwise "reopen" would do nothing after the cutoff |
| D21 | 2026-11-11 | Members see others' live requests (name, work, share). **Decision notes and compatibility flags** are visible only to the requester and kiln masters; timeline entries never include decision notes | Privacy without hiding who's in |
| D22 | 2026-11-11 | Requests can be approved/declined until the kiln starts (proposed … loaded) | Kiln master often decides at loading time |
| D24 | 2026-11-11 | **Multiple kilns supported** (owner request). `/api/options` lists every kiln (active first); creating a firing takes `kiln_id`, optional while only one kiln is active. The kiln name is shown only when there are 2+ kilns. Kiln QR codes will carry the kiln (`/kiln?k=<id>`). Adding/editing kilns belongs on `/admin/settings` (M4) | Future-proofing |
| D25 | 2026-11-11 | **Back-dating** (kiln masters only): up to 7 days back, never more than 5 min in the future, never before the firing's last recorded step. Others' times are always "now" | §6/§12 accuracy without letting the timeline get scrambled |
| D26 | 2026-11-11 | Kiln QR flow lives at `/kiln` (pick list) and `/kiln/:id` (actions), so a single relevant firing redirects to a real URL | Refresh-safe and shareable on a phone |
| D27 | 2026-11-11 | Admins can't remove their own admin flag in the app | Avoids locking yourself out (`ADMIN_USER_IDS` would also re-grant at next login) |
| D28 | 2026-11-11 | **Worker renders the main message at send time** from the firing's current state; outbox rows only carry the firing id (+ thread text). DMs carry their text | Coalesces bursts of edits and never posts stale state |
| D29 | 2026-11-11 | Thread-reply text rides on the `channel_upsert` row (no new outbox kind) | The thread hangs off the main message, so they're delivered together, in order |
| D30 | 2026-11-11 | DMs go to the user's first account's platform; the actor is never DMed about their own action | §11 "platform they signed in with"; avoids self-notifications |
| D31 | 2026-11-11 | Changing the channel in settings only affects firings without a message yet | Keeps edit-in-place simple |
| D32 | 2026-11-11 | `NOTIFICATIONS_DRY_RUN` logs rendered messages instead of sending | Develop/demo notifications without bot tokens |
| D33 | 2026-11-11 | **One "requests closed" job per cutoff**, tracked by new `firings.requests_close_handled_at` (migration 003). It refreshes the channel message and sends kiln masters one DM: under-filled (below the threshold) takes priority, otherwise "needs confirming" if still proposed/on hold | §10 lets the unconfirmed reminder ride along with the under-filled alert; one DM instead of two |
| D34 | 2026-11-11 | Reschedule and reopen/close requests clear the close marker (and the under-filled marker), so the job runs again for the new cutoff. A manual close counts as a close | Matches "requests just closed" whichever way they closed |
| D35 | 2026-11-11 | A firing created after its own cutoff (kiln master, short notice) closes quietly: no alert DMs | It was never open for requests; alerting its creator is noise |
| D36 | 2026-11-11 | Missed-start reminders go to kiln masters; missed-unload reminders go to kiln masters **plus the assigned loader** (the loader part is superseded by #15). Reminders don't touch the channel message | §10 table |
| D37 | 2026-11-11 | Migration 003 marks firings that are already past their cutoff or overdue as handled | Avoids a burst of stale alerts on the first tick after deploying |
| D38 | 2026-11-11 | `/upcoming` shows proposed/on hold/confirmed firings for the next `public_page_days_ahead` days, plus anything in the kiln. It refreshes every minute. "On hold" is shown as "Waiting for more work" | §9A; wheel-side screen |
| D39 | 2026-11-11 | `/calendar.ics` covers the last 30 days to `public_page_days_ahead` ahead, excluding cancelled firings. Events run from the start to the unload time when one is known (actual → estimated → start + program duration), otherwise they're a 1-hour marker. Proposed/on hold firings are `TENTATIVE` | Firings that haven't happened don't fill a day in people's calendars |
| D40 | 2026-11-11 | A change to the request cutoff applies to firings proposed or rescheduled afterwards (`requests_close_at` is stored per firing) | Firings people already planned around don't shift |
| D41 | 2026-11-11 | Printable QR codes on `/admin/qr`, drawn in the browser with `uqr`, using `APP_BASE_URL`. With more than one active kiln, each kiln QR is `/kiln?k=<id>` | No server QR dependency; warns when the base URL is localhost |
| D42 | 2026-11-11 | At least one kiln must stay active | New firings need a kiln (`pickKiln`) |
| D43 | 2026-11-14 | Membership and the Kiln Master role are re-checked hourly with the bot tokens (`jobs.RecheckMembers`) for accounts with a live session or the kiln master flag. Leavers are signed out; a lookup that fails changes nothing. Off with `DEV_LOGIN` or without a bot token | Sessions last 7 days; without this, people who left kept access until expiry (security review) |
| D44 | 2026-11-14 | Admins can sign a user out everywhere (`POST /api/admin/users/{id}/sign-out`). It doesn't block signing in again; remove the person in Discord/Slack for that | Lost phone, shared device |
| D45 | 2026-11-14 | Sign-in start/callback are limited to 30 per minute per client IP, in memory. Expired sessions and OAuth states are deleted on each jobs tick | Starting sign-in writes a row without a login |
| D46 | 2026-11-14 | Plain messages (DMs, thread replies) are escaped at send time: Slack `& < >`, Discord `[ ] \` | Member-written titles and reasons could render as disguised links |
| D23 | 2026-11-11 | Datetimes are entered and shown in `settings.timezone`, not the phone's timezone. Binary embeds `time/tzdata` | Everyone sees studio wall time |
| D47 | 2026-11-14 | Adopt the **Kiln Control** design system: plain `kc-*` CSS classes and tokens in `web/src/styles/`, with Tailwind kept for layout utilities. Fonts (Barlow, Barlow Condensed, Share Tech Mono) are self-hosted in `web/public/fonts/` | Owner supplied the system. Self-hosting keeps the server's strict CSP and removes external font requests. 56px tap targets (48px compact) suit gloved studio hands |
| D48 | 2026-11-14 | Slack main messages carry the firing's status colour by moving their Block Kit into a **legacy attachment** whose `color` is `statusColors[status]` — the same value Discord already puts on its embed. Cards with no colour get no bar. The plain text moves to the attachment's `fallback`; there is no top-level `text`, which Slack would otherwise show above the attachment | Block Kit has no colour of its own; the attachment is the only place Slack renders one. Otherwise "firing" and "on hold" looked identical on Slack while Discord showed a red/amber strip |

---

## Open questions (spec §17), current defaults

**Questions 1–5 change code logic.** M1 was built on the defaults (D16); changing them later is small. The rest are just values in settings.

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
| 13 | Unloaded, ready for pickup and completed: separate steps? | **One step** (`completed`, labelled "Unloaded: ready for pickup"). It sets `unloaded_at`, `ready_for_pickup_at` and `completed_at` to the same time, sends pickup DMs, and the assigned loader can take it. Old `unloaded` / `ready_for_pickup` firings are migrated to `completed` (migration 004). Owner's call after testing, to be less fussy. | Post-M4 |
| 14 | Firing start: exact time or day? | **Day only.** A firing is booked for a calendar day in the studio timezone (`scheduled_date`, a DATE). No start time is promised; the kiln runs sometime that day. Rules count from the start of the day (local midnight): notice, request cutoff. Missed-start reminders fire after the day ends plus the grace. Calendar: unstarted firings are all-day events. Real moments (started, cooling, unloaded) stay full timestamps. Migration 005 converts existing start times to their local day. | Post-M4 |
| 15 | Loaders: a separate role? | **No.** Nobody is assigned as a loader and there's no self-loader flag. Anyone signed in can scan the kiln code to say they've loaded (`loading`, `loaded`) or unloaded (`completed`) their work. Only kiln masters start the kiln and mark cooling. Missed-unload reminders go to kiln masters. Removed `firings.loader_user_id` and `users.is_self_loader` (migration 006), the loader assignment endpoint and UI, and the People page's self-loader toggle (People is admin-only now). Owner's call. | Post-M4 |
