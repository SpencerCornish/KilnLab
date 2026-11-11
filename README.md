# KilnLab

Firing coordinator for a shared studio kiln used by two communities: ArtLab (Discord) and BMS (Slack).
Members propose firings and request space in them. Kiln masters approve, run and track each firing,
mostly from a phone after scanning a QR code on the studio wall. Product spec: [KILN_APP_SPEC.md](KILN_APP_SPEC.md).

## Stack

The structure follows BookLab, with current dependency versions.

- **Backend:** Go 1.26 (`cmd/server`, `cmd/seed`, plus `internal/{api,auth,config,db,firing,notify,policy,webembed}`), chi router
- **Database:** PostgreSQL 18 via pgx. Migrations are embedded and run on startup (golang-migrate). Queries are generated with **sqlc** from `internal/db/queries/*.sql`.
- **Frontend:** React 19 + TypeScript 7 SPA (Vite 8, Tailwind 4, react-router 8), embedded into the Go binary
- **Auth:** Discord OAuth2 (ArtLab guild membership + Kiln Master role) and Sign in with Slack (OIDC, BMS workspace check), using `golang.org/x/oauth2`. Sessions are stored in the DB behind a cookie, with double-submit CSRF.
- **Deploy:** one Docker image plus Postgres (`docker-compose.prod.yml`) behind a TLS reverse proxy

### Deviations from the spec's default stack (§3)

| Spec default | KilnLab | Why |
|---|---|---|
| Next.js server rendering | Go API + embedded React SPA | Matches BookLab, the owner's existing app |
| Auth.js | `x/oauth2` + hand-rolled membership checks (§5) | Go stack |
| `/api/cron/tick` hit by external cron | In-process ticker goroutine (M4) | Single binary, no external cron to configure. Jobs stay idempotent through the `*_sent_at` columns. |

## Local development

```bash
make install-tools   # asdf: Go, Node, pnpm from .tool-versions
make setup           # creates .env (DEV_LOGIN=true) and installs web deps
make dev             # Postgres on :5433, API on :8080, Vite on :5173
```

Run `make seed` once for sample firings. With `NOTIFICATIONS_DRY_RUN=true` (the default in `.env.example`), Discord and Slack messages are written to the server log instead of being sent; admins can also see them on `/admin`. With `DEV_LOGIN=true`, sign in as one of the seeded users (Kim the kiln master, Sam the self-loader, Alice, Bob on BMS, Carol) by typing their name.
The login page shows a form for signing in as any fake user and role, with no Discord or Slack app needed.
To test real OAuth through Vite, set `APP_BASE_URL=http://localhost:5173` so the callbacks come back via the Vite proxy.

```bash
make test        # Go unit + Postgres integration tests (uses kilnlab_test DB)
make lint        # go vet, gofmt, tsc
make sqlc        # regenerate internal/db after editing migrations or queries
make build       # SPA + ./kilnlab binary
```

Dev Postgres listens on host port **5433**, so it can run alongside BookLab's on 5432.

## Setting up Discord and Slack

**Discord (ArtLab)**: https://discord.com/developers/applications
1. Create an application. Under **OAuth2**, add the redirect `https://<APP_BASE_URL>/api/auth/discord/callback`.
2. Copy the Client ID and Secret into `DISCORD_CLIENT_ID` and `DISCORD_CLIENT_SECRET`.
3. Enable Developer Mode in Discord, then right-click to copy IDs: the server → `ARTLAB_GUILD_ID`, the Kiln Master role → `DISCORD_KILN_MASTER_ROLE_ID`.
4. Notifications: under **Bot**, copy the token into `DISCORD_BOT_TOKEN`. Invite the bot (OAuth2 → URL Generator, scope `bot`) with **Send Messages, Embed Links, Create Public Threads, Send Messages in Threads, Read Message History**. Put the channel ID in `DISCORD_CHANNEL_ID`. Members only get DMs if they allow DMs from server members.

**Slack (BMS)**: https://api.slack.com/apps
1. Create an app in the BMS workspace. Under **OAuth & Permissions**, add the redirect `https://<APP_BASE_URL>/api/auth/slack/callback`.
2. User token scopes: `openid profile email`. Bot token scopes: `users:read` (guest check at login), `chat:write` and `im:write` (notifications). Invite the bot to the notifications channel (`/invite @YourApp`) and put the channel ID in `SLACK_CHANNEL_ID`.
3. Install the app to the workspace. Set `SLACK_CLIENT_ID`, `SLACK_CLIENT_SECRET`, `SLACK_BOT_TOKEN`, and `SLACK_TEAM_ID` (the `T…` ID).

**Admins**: set `ADMIN_USER_IDS` to your provider ID, e.g. `discord:123456789012345678`. You can also find it by signing in once and running `SELECT provider, provider_user_id FROM accounts;`.

## Production

```bash
cp .env.example .env    # fill in real values; set DEV_LOGIN=false, APP_BASE_URL=https://…
docker compose -f docker-compose.prod.yml up -d --build
```

The app binds to `127.0.0.1:8080`. Put Caddy or nginx in front for TLS. The server refuses to start if `DEV_LOGIN` is on while `APP_BASE_URL` is https.

## Mobile sign-in notes

On phones, OAuth can bounce through the Discord or Slack app and return in a different browser.
OAuth state is therefore stored server-side, and the sign-in completes in whichever browser receives the callback.
A browser-binding cookie is checked when present, which blocks state reuse inside one browser.
The trade-off is a weaker defence against login CSRF; that's acceptable for this app.
