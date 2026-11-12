# Kiln Firing Coordinator — Build Spec (v1)

> Handoff document for a coding agent. Read this whole file before writing code.
> Where something is marked **[CONFIRM]**, ask the owner before implementing.

---

## 0. Instructions for the agent

1. **Confirm the stack first.** The owner has already built other apps (darkroom booking, access control, website). Ask what stack/hosting those use and prefer matching it. If the owner has no preference, use the default in §3.
2. **Build v1 only** (§2). Do not build billing, energy tracking, maintenance tracking, or structured firing logs yet — but do store the data called out in §12 so they're easy later.
3. **Mobile-first.** Almost every real interaction happens on a phone after scanning a QR code on the studio wall.
4. **Keep it boring.** One app, one database, server-rendered pages where possible, minimal client JS. This is a small-community tool that has to keep working with little maintenance.
5. **Test the core logic:** the firing state machine (§6), permissions (§4), fullness math (§7), and scheduled jobs (§10) need unit tests.
6. Work in the milestones in §14, checking in with the owner at the end of each.

---

## 1. Context

- A ceramics studio has **one shared electric kiln** (design for possibly more later — see `kilns` table).
- Two communities use it:
  - **ArtLab** — communicates on **Discord**.
  - **BMS** — communicates on **Slack**.
- A **kiln master** (one or more people) runs firings. Kiln masters are **always on the ArtLab Discord** and have a Discord role.
- Today, coordination happens over DMs and is messy. **The goal is fuller firings**: make it easy to see upcoming firings, see how much room is left, and add work to an existing firing instead of firing half-empty loads.
- The app is **not connected to the kiln**. Status is updated by people, mainly by scanning a QR code mounted next to the kiln.
- Physical access control already prevents non-members from using the kiln, so the app does **not** need strict membership revocation logic.
- Timezone: **America/Denver**.

---

## 2. Scope

### v1 (build this)
- Discord + Slack login
- Propose a firing; kiln master approves / holds / reschedules / cancels
- Request space in a firing; approve/decline
- Fullness calculated from approved shares, with kiln master override
- Minimum notice for proposals; automatic request cutoff
- Firing status lifecycle, updated via the in-app UI and the kiln-wall QR code
- "Start firing" captures program + estimated duration → posts estimated unload time
- Notifications to one Discord channel and one Slack channel (one message per firing, edited in place)
- Direct message to participants when work is ready for pickup
- Public read-only "upcoming firings" page (wheel-wall QR code) + iCal feed
- Scheduled jobs: cutoff, under-filled alert, missed-scan reminders
- Admin settings page

### Next (do not build yet)
- Structured firing logs (peak temp, cone pack results, error codes, before/after photos)
- Flagging problems on a specific participant's work
- Firing count / element hours / maintenance reminders
- Per-participant pickup tracking and nudges for unclaimed work

### Later (do not build yet)
- Energy use per firing
- Billing by kiln share, split by community
- Off-peak start-time suggestions

---

## 3. Default stack (only if owner has no preference) [CONFIRM]

- **TypeScript + Next.js (App Router)**, server actions / route handlers
- **Postgres** with **Drizzle** (or Prisma) migrations
- **Auth.js (NextAuth)** with Discord and Slack providers (custom checks in §5)
- **Job runner:** a single `/api/cron/tick` endpoint called every 5 minutes by the host's cron (or `pg-boss` if a long-running worker is available). Jobs must be idempotent.
- **Discord:** a bot user via REST (no gateway connection needed) — `discord.js` REST client or plain `fetch`
- **Slack:** a Slack app with a bot token (`chat:write`, `users:read`, `im:write`) via `@slack/web-api`
- **iCal:** `ical-generator`
- **QR codes:** generate once (any generator) — they are static URLs (§9)

---

## 4. Roles and permissions

Roles are derived at login and re-checked at each login (§5). A user can have multiple.

| Role | How determined |
|---|---|
| **Member** | Signed in via Discord (in ArtLab guild) or Slack (in BMS workspace) |
| **Kiln master** | Has the configured **Kiln Master role** in the ArtLab Discord guild |
| **Admin** | Allowlist of user IDs in settings/env (the owner) — can also be kiln master |

### Permission matrix

| Action | Member | Kiln master | Admin |
|---|:-:|:-:|:-:|
| View public page / iCal | ✓ (no login needed) | ✓ | ✓ |
| Propose a firing | ✓ | ✓ | ✓ |
| Request space in a firing | ✓ | ✓ | ✓ |
| Edit/withdraw own request (before cutoff) | ✓ | ✓ | ✓ |
| Approve/decline requests | proposer of that firing* | ✓ | ✓ |
| Confirm / hold / release / reschedule / cancel firing | | ✓ | ✓ |
| Override fullness; close/reopen requests | | ✓ | ✓ |
| Bypass minimum notice | | ✓ | ✓ |
| Mark **Loading** / **Loaded** (scan the kiln code) | ✓ | ✓ | ✓ |
| Mark **Firing (Start)** | ✗ never | ✓ | ✓ |
| Mark **Cooling** | | ✓ | ✓ |
| Mark **Unloaded: ready for pickup** (completes the firing) | ✓ | ✓ | ✓ |
| Correct a status timestamp after the fact | | ✓ | ✓ |
| Manage people (admin flag) | | | ✓ |
| Edit settings | | | ✓ |

\* **[CONFIRM]** whether the firing's proposer can approve requests, or only kiln masters. Default: kiln master only; proposer is notified.

**Only kiln masters can start the kiln.** Enforce this server-side, not just in the UI.

---

## 5. Authentication

Two providers. A user record is keyed by `(provider, provider_user_id)`.

### Discord (ArtLab)
- OAuth2 scopes: `identify guilds.members.read`
- After login, call `GET /users/@me/guilds/{ARTLAB_GUILD_ID}/member` with the user's access token.
  - 404 / error → reject login ("You need to be in the ArtLab Discord").
  - Read `roles[]`. If it contains `DISCORD_KILN_MASTER_ROLE_ID` → kiln master.
- Store display name (guild nickname if set, else global name) and avatar.

### Slack (BMS)
- **Sign in with Slack** (OpenID Connect), scopes `openid profile email`.
- Pass `team=SLACK_TEAM_ID` on the authorize URL to preselect the workspace.
- **Verify server-side** that the `https://slack.com/team_id` claim equals `SLACK_TEAM_ID`. Reject otherwise.
- Optional (nice to have): using the bot token, call `users.info` and reject or limit guests (`is_restricted` / `is_ultra_restricted`). **[CONFIRM]** whether Slack guests should be allowed.

### Sessions
- Re-derive roles on every login. Session lifetime ~7 days.
- Kiln-master role changes take effect at next login; that is acceptable.
- **Test the full login flow from a phone after scanning a QR code** (iOS Safari and Android Chrome), including when the Discord/Slack mobile app is installed. OAuth on mobile can bounce through native apps or return to a different browser. This flow must be smooth or the kiln-wall QR will not get used.

### Known limitation (accept for v1)
A person in both communities could end up with two user records (one per provider). Don't build account linking yet; just don't make it hard to add later (keep a separate `users` table and an `accounts` table).

---

## 6. Firing lifecycle (state machine)

```
            ┌──────────── hold ───────────┐
            ▼                             │
 PROPOSED ──confirm──▶ CONFIRMED ──▶ LOADING ──▶ LOADED ──start──▶ FIRING ──▶ COOLING ──▶ COMPLETED (unloaded + ready for pickup)
    │  ▲                  │  ▲
    │  └──── release ─────┤  │
    ▼                     ▼  │
  ON_HOLD ◀──── hold ─────┘  │
    │
    ▼
 CANCELLED  (from PROPOSED, ON_HOLD, CONFIRMED, LOADING, LOADED)
```

| Status | Meaning | Who moves it here |
|---|---|---|
| `proposed` | Someone proposed it; open for requests | Any member (on create) |
| `on_hold` | Kiln master paused it (e.g. not enough work yet). Still open for requests unless closed. Has `hold_reason`. | Kiln master |
| `confirmed` | Kiln master approved; will run on scheduled date | Kiln master |
| `loading` | Being loaded | Anyone signed in (kiln QR) |
| `loaded` | Loaded, waiting for kiln master to start | Anyone signed in (kiln QR) |
| `firing` | Kiln is running. Requires `program` and `estimated_duration` → computes `estimated_unload_at` | **Kiln master only** |
| `cooling` | Kiln off, cooling | Kiln master |
| `completed` | Work removed from the kiln, ready for pickup, and done. One step: sets `unloaded_at`, `ready_for_pickup_at` and `completed_at`, and triggers pickup DMs. | Anyone signed in (kiln QR) |
| `cancelled` | Won't run. Has `cancel_reason`. | Kiln master |

Rules:
- Every transition writes a `firing_events` row (who, from, to, at, note). `at` defaults to now; kiln masters may set an earlier time ("actually started at 9:15").
- Invalid transitions are rejected server-side.
- `release` from `on_hold` returns to `proposed` (or `confirmed` if it had been confirmed before being held) and may change `scheduled_date`.
- Rescheduling (changing `scheduled_date`) is allowed in `proposed`, `on_hold`, `confirmed` and notifies all participants.
- Cancelling notifies all participants.
- Skipping states is allowed for kiln masters (e.g. `confirmed` → `firing` if they forgot to scan loading); intermediate states are just not recorded. Anyone signed in can skip to `loading`, `loaded` or `completed`, but not into `firing`.

### Requests window
- Requests are open while status ∈ {`proposed`, `on_hold`, `confirmed`} **and** now < `requests_close_at` **and** `requests_closed_manually` is false.
- `requests_close_at` = start of `scheduled_date` (local midnight, studio timezone) − `settings.request_cutoff_hours` (default 12h — **[CONFIRM]**).
- Kiln master can close early or reopen.

### Minimum notice
- On create, the start of `scheduled_date` (local midnight) must be ≥ now + `settings.min_notice_hours` (default 48h — **[CONFIRM]**). Kiln masters can bypass. A day that has already ended can't be booked.

---

## 7. Kiln share and fullness

- Every proposal and request includes:
  - `description` — free text ("4 mugs, 2 medium bowls, 1 tall vase")
  - `estimated_share_pct` — integer 1–100
  - `cone` — the cone the work is meant for (select from settings list)
  - `clay_body` — optional free text
- Under every share input, show the **kiln size footnote** from settings, e.g.:
  > *Our kiln is [X]" wide × [Y]" deep, about [N] shelves. One full shelf ≈ [Z]%.*
  **[CONFIRM]** kiln dimensions and shelf count with the owner/kiln master.
- **Fullness** = sum of `estimated_share_pct` of the proposer's own entry + all `approved` requests, capped for display at 100%.
- Kiln master can set `fullness_override_pct`; when set, display it instead (show "set by kiln master").
- Display "room left" = 100 − fullness.
- If a new request would push calculated fullness over 100%, allow it but warn ("This may not fit — the kiln master will decide").

### Compatibility check
- If a request's `cone` ≠ firing's `cone`, or firing type is bisque and request indicates glazed work, show a **warning** to the requester and a **flag** on the request for the kiln master. Do not hard-block — the kiln master decides.

---

## 8. Data model

Suggested tables (adjust naming to stack conventions). All timestamps stored in **UTC**; display in America/Denver.

**users**
- `id`, `display_name`, `avatar_url`, `community` (`artlab` | `bms`), `is_admin` (bool), `created_at`, `last_login_at`

**accounts**
- `id`, `user_id`, `provider` (`discord` | `slack`), `provider_user_id`, `provider_team_or_guild_id`, `is_kiln_master` (bool, refreshed at login), tokens as required by auth library

**kilns**
- `id`, `name`, `interior_width_in`, `interior_depth_in`, `interior_height_in`, `shelf_count`, `share_per_shelf_pct`, `active`
- Seed one kiln.

**firings**
- `id`, `kiln_id`, `proposed_by_user_id`
- `firing_type` (`bisque` | `glaze` | `other` — from settings), `cone`, `title` (optional), `notes`
- `scheduled_date` (date): the day the firing is booked for. **No start time is promised.** Times below are real moments, recorded as they happen.
- `status` (enum from §6), `hold_reason`, `cancel_reason`
- `proposer_description`, `proposer_share_pct`, `proposer_cone`, `proposer_clay_body`
- `fullness_override_pct` (nullable)
- `requests_close_at`, `requests_closed_manually` (bool)
- `program` (nullable text), `estimated_duration_minutes` (nullable), `started_at`, `estimated_unload_at`, `cooling_at`, `unloaded_at`, `ready_for_pickup_at`, `completed_at`
- `underfilled_alert_sent_at`, `missed_start_reminder_sent_at`, `missed_unload_reminder_sent_at` (job idempotency)
- `created_at`, `updated_at`

**firing_requests**
- `id`, `firing_id`, `user_id`
- `description`, `estimated_share_pct`, `cone`, `clay_body`
- `status` (`pending` | `approved` | `declined` | `withdrawn`), `decision_note`, `decided_by_user_id`, `decided_at`
- `compatibility_warning` (nullable text)
- `created_at`, `updated_at`

**firing_events** (audit log / timeline)
- `id`, `firing_id`, `actor_user_id`, `type` (`status_change`, `rescheduled`, `request_approved`, `fullness_override`, …), `from_status`, `to_status`, `occurred_at` (may be back-dated), `recorded_at`, `note`, `source` (`web` | `kiln_qr` | `system`)

**notification_messages** (one per firing per platform, for edit-in-place)
- `id`, `firing_id`, `platform` (`discord` | `slack`), `channel_id`, `message_id` (Discord) / `ts` (Slack), `last_rendered_hash`, `updated_at`

**notification_outbox** (reliable delivery)
- `id`, `kind` (`channel_upsert` | `dm`), `platform`, `payload` (json), `status` (`pending` | `sent` | `failed`), `attempts`, `last_error`, `next_attempt_at`, `created_at`

**settings** (single row or key/value)
- `timezone` (default `America/Denver`)
- `min_notice_hours`, `request_cutoff_hours`
- `firing_types[]`, `cones[]`, `programs[]` (common controller programs, with optional default duration)
- `discord_channel_id`, `slack_channel_id`
- `missed_start_grace_minutes` (default 120), `missed_unload_grace_minutes` (default 180)
- `public_page_days_ahead` (default 30)

---

## 9. QR code flows

There are exactly **two physical QR codes**. Both are static URLs.

### A. Wheel QR → `/upcoming` (public)
- No login required to view.
- Lists firings in `proposed`, `on_hold`, `confirmed` (and optionally in-progress ones) for the next `public_page_days_ahead` days.
- Each card: day (no time), type, cone, status badge ("Proposed", "Waiting for more work", "Confirmed"), **room left %** with a bar, requests-close time, and participant count. Don't show participant names publicly — **[CONFIRM]**.
- "Request space" → login (if needed) → request form (§7) → back to the firing page.
- "Propose a firing" button.
- Link to the iCal feed.

### B. Kiln QR → `/kiln` (login required)
1. If not logged in → login → return to `/kiln`.
2. Determine the **relevant firing(s)**:
   - Any firing in `loading`, `loaded`, `firing`, `cooling`; plus
   - `confirmed` firings whose scheduled day starts within ±24h.
   - If exactly one → go to it. If several → short pick list. If none → "No active firing" + link to schedule.
3. Show the firing summary and **only the next valid action buttons for this user's role** (§4, §6). Large touch targets.
   - Kiln masters see a prominent **START FIRING** button when status is `loaded` (or `confirmed`/`loading` — skipping allowed).
4. Tapping an action opens a **confirm screen**: firing, action, time (editable by kiln masters for back-dating), and action-specific fields:
   - **Loading** — nothing required.
   - **Loaded** — optional note.
   - **Start firing** — `program` (select from settings or free text) and `estimated_duration` (prefilled from program default) → shows computed estimated unload time before confirming.
   - **Cooling** — optional note.
   - **Unloaded: ready for pickup** (completed) — optional note.
   - **Ready for pickup** — nothing required; warns that participants will be messaged.
5. On confirm: transition, write `firing_events` with `source = kiln_qr`, enqueue notifications, show success with what got posted.
6. Users without permission for an action see a clear message, e.g. "Only kiln masters can start the kiln."

All the same actions must also be available from the normal firing page in the web UI (for fixing missed scans from home).

---

## 10. Scheduled jobs

Single tick every ~5 minutes. Each job is idempotent via the `*_sent_at` columns.

| Job | Condition | Action |
|---|---|---|
| **Close requests** | now ≥ `requests_close_at` and status ∈ {proposed, on_hold, confirmed} | Update the channel message ("Requests closed"). |
| **Under-filled alert** | Requests just closed and fullness < 50% (**[CONFIRM]** threshold) and `underfilled_alert_sent_at` is null | DM kiln master(s): "Glaze firing Thu is at 30% and requests are closed." Link to firing page with quick actions: **run anyway (confirm)**, **postpone (reschedule)**, **reopen requests**, **cancel**. (Merge is a stretch goal — see §15.) No automatic decision. |
| **Unconfirmed reminder** | status ∈ {proposed, on_hold} and `requests_close_at` passed | Include in the under-filled alert or send a separate "needs confirmation" DM to kiln masters. |
| **Missed start** | status ∈ {confirmed, loading, loaded} and now > end of `scheduled_date` (local midnight) + `missed_start_grace_minutes` and reminder not sent | DM kiln master(s): "Thu glaze firing hasn't been marked started." |
| **Missed unload** | status ∈ {firing, cooling} and now > `estimated_unload_at` + `missed_unload_grace_minutes` and reminder not sent | DM kiln master(s). |
| **Outbox delivery** | `notification_outbox` rows pending and due | Send; on failure, exponential backoff, max ~8 attempts, then `failed`. |

Who receives kiln-master DMs: all users currently flagged kiln master. **[CONFIRM]** whether there should be a single "on duty" kiln master per firing instead.

---

## 11. Notifications

### Principles
- **One message per firing per platform**, created on proposal and **edited in place** on every change. No channel spam.
- For important moments (proposed, confirmed, started, ready for pickup, cancelled, rescheduled) also post a short **threaded reply** under the main message so people following the thread get notified. **[CONFIRM]** — default on for started / ready for pickup / cancelled / rescheduled.
- Notification failures must **never** block a state change. All sends go through `notification_outbox`.

### Discord
- Bot posts to `discord_channel_id` via `POST /channels/{id}/messages`; store `message_id`; edit via `PATCH /channels/{id}/messages/{message_id}`. Threads: create a thread from the message on first threaded reply.
- DMs: `POST /users/@me/channels` with `recipient_id` → send to returned channel. Users must share the guild with the bot; handle "cannot DM" (user privacy settings) gracefully — log it, don't retry forever.
- Use embeds for the main message.

### Slack
- Bot posts via `chat.postMessage` to `slack_channel_id`; store `ts`; edit via `chat.update`. Threaded replies via `thread_ts`.
- DMs: `chat.postMessage` with `channel = <user's Slack ID>`.
- Use Block Kit for the main message.
- Note: Slack **incoming webhooks cannot edit messages** — a bot token is required.

### Main message content (both platforms)
```
🔥 Cone 6 Glaze — Thu Nov 13, 9:00am
Status: Proposed · ~75% full · room for ~25%
Proposed by Cody · 3 participants
Requests close Wed 8:00pm
[View / request space →]
```
Variations by status:
- On hold: "⏸ Waiting for more work — <hold_reason>"
- Firing: "Firing now · started 9:15am · est. unload Fri 3:00pm"
- Ready for pickup: "✅ Ready for pickup"
- Cancelled: "❌ Cancelled — <reason>" (strike or grey out)

### Direct messages
- **Ready for pickup** → each participant with an approved request + the proposer.
- **Request approved / declined** → requester (with note).
- **Rescheduled / cancelled / put on hold** → all participants.
- **Kiln master alerts** → from §10.
- A participant gets DMs on the platform they signed in with.

### Pinging people in the channel
- Don't @mention participants in the main channel message by default (privacy + noise). DMs cover individual notice.

---

## 12. Data to capture now for later features

Even though billing/energy/maintenance are out of scope, make sure v1 records:
- Each participant's `estimated_share_pct` and final approved status
- `fullness_override_pct`
- Accurate `started_at`, `cooling_at`, `unloaded_at` (with back-dating support)
- `program` and `estimated_duration_minutes`
- `kiln_id` on every firing
- `community` on every user

---

## 13. Other pages

- **`/` (logged in)** — upcoming firings (same as `/upcoming` but with actions), "My firings" (proposed by me / I have work in), quick "Propose a firing".
- **`/firings/new`** — propose form: type, cone, day (no time), own description, own share %, clay body, notes. Enforces min notice (kiln masters can bypass).
- **`/firings/:id`** — details, fullness bar, request list (kiln masters see full details + compatibility flags + approve/decline), timeline from `firing_events`, status action buttons for permitted users, kiln-master controls (confirm, hold w/ reason, release, reschedule, cancel w/ reason, override fullness, close/reopen requests).
- **`/kiln`** — kiln QR flow (§9B).
- **`/upcoming`** — public page (§9A).
- **`/calendar.ics`** — public iCal feed of non-cancelled upcoming firings (title like "Cone 6 Glaze (75% full)").
- **`/admin/users`** — admins: toggle admin.
- **`/admin/settings`** — admin: everything in `settings`, kiln dimensions.

UI: mobile-first, high contrast, large tap targets. Studio hands may be dirty or gloved; keep forms short.

---

## 14. Milestones and acceptance criteria

**M0 — Scaffold & auth**
- Project set up, DB migrations, seed one kiln and default settings.
- Discord login with guild membership + kiln master role detection.
- Slack login with workspace verification.
- Deployed to a staging URL; **login verified from phones** via QR scan.

**M1 — Firings & requests**
- Propose, view, request space, approve/decline, withdraw.
- Fullness math + override; compatibility warnings; kiln-size footnote.
- Min notice + request cutoff enforcement.
- Kiln master: confirm, hold/release, reschedule, cancel, close/reopen requests.
- Unit tests for state machine, permissions, fullness.

**M2 — Status lifecycle & kiln QR**
- All status transitions with permissions and back-dating.
- `/kiln` flow end-to-end on a phone, including the pick list case.
- Start firing → program + duration → estimated unload time.

**M3 — Notifications**
- Discord + Slack main message created and edited in place.
- Threaded replies for key events.
- DMs for approval/decline, reschedule/cancel/hold, ready for pickup.
- Outbox with retries; failures don't block actions.

**M4 — Jobs, public page, calendar**
- Cron tick with all §10 jobs, idempotent, tested.
- `/upcoming` public page; `/calendar.ics`.
- Admin settings page.

### End-to-end acceptance scenario
1. **Mon 10am** — Cody (Discord) proposes Cone 6 Glaze for Thu 9am, 40% share. Discord + Slack messages appear.
2. **Mon pm** — Annabelle and Emily request 20% and 15%. Kiln master approves both. Messages update to ~75% full; no new top-level posts.
3. **Tue** — A BMS member (Slack) scans the wheel QR, logs in with Slack, requests 10% with cone 10 clay. Compatibility warning shows; kiln master declines with a note; member gets a Slack DM.
4. **Wed 9pm** — Requests auto-close (cutoff 12h). Kiln master confirms and overrides fullness to 85%.
   - *Alternate:* if fullness were 30%, kiln master gets the under-filled DM, chooses **postpone** one week; all participants are notified.
5. **Thu 8:45am** — Emily scans kiln QR → Loading → Loaded.
6. **Thu 9:15am** — Kiln master scans kiln QR → Start firing → selects program, duration 30h → posts "est. unload Fri 3:15pm". Emily attempting Start is refused.
7. **Thu evening** — Kiln master marks Cooling (back-dated to 6pm).
8. **Fri 3:30pm** — Unloaded → Ready for pickup. Cody, Annabelle, Emily each get a DM on their platform.
9. **Missed-scan check** — In a separate test, a confirmed firing passes its start time + grace with no scan → kiln masters get a reminder exactly once.

---

## 15. Stretch goals (only if v1 is done and owner agrees)
- **Merge firings**: kiln master merges firing B into A — moves B's approved requests to A, cancels B with reason "merged into A", notifies B's participants.
- Account linking (same person on Discord and Slack).
- "Notify me when a cone X firing is proposed" subscriptions.

---

## 16. Environment variables

```
DATABASE_URL=
APP_BASE_URL=
SESSION_SECRET=
TZ_DISPLAY=America/Denver

# Discord
DISCORD_CLIENT_ID=
DISCORD_CLIENT_SECRET=
DISCORD_BOT_TOKEN=
ARTLAB_GUILD_ID=
DISCORD_KILN_MASTER_ROLE_ID=
DISCORD_CHANNEL_ID=          # can also live in settings

# Slack
SLACK_CLIENT_ID=
SLACK_CLIENT_SECRET=
SLACK_BOT_TOKEN=
SLACK_TEAM_ID=
SLACK_CHANNEL_ID=            # can also live in settings

# Admin
ADMIN_USER_IDS=              # comma-separated provider IDs for bootstrap

# Cron
CRON_SECRET=                 # required header for /api/cron/tick
```

---

## 17. Open questions for the owner [CONFIRM]

1. Stack and hosting (match existing apps?).
2. Kiln interior dimensions, shelf count, and "one shelf ≈ X%".
3. Minimum notice (default 48h) and request cutoff (default 12h before start).
4. Under-filled threshold for the kiln master alert (default 50%).
5. Can a firing's proposer approve requests, or only kiln masters? (Default: kiln masters only.)
6. Allow Slack guests? (Default: no.)
7. Show participant names on the public page? (Default: count only.)
8. Firing types, cone list, and common controller programs with typical durations.
9. Discord and Slack channel IDs for notifications.
10. Should `completed` happen automatically N days after ready-for-pickup, or manually?
11. One "on duty" kiln master per firing, or alert all kiln masters?
