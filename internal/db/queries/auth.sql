-- name: GetAccountByProvider :one
SELECT * FROM accounts
WHERE provider = $1 AND provider_user_id = $2;

-- name: CreateUser :one
INSERT INTO users (display_name, avatar_url, community, is_admin)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateUserOnLogin :one
-- Admin is only ever granted here (bootstrap allowlist), never revoked, so an
-- admin toggled on in the app stays admin.
UPDATE users
SET display_name  = $2,
    avatar_url    = $3,
    is_admin      = is_admin OR sqlc.arg(grant_admin)::boolean,
    last_login_at = now()
WHERE id = $1
RETURNING *;

-- name: CreateAccount :one
INSERT INTO accounts (user_id, provider, provider_user_id, provider_team_or_guild_id, is_kiln_master)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateAccountOnLogin :exec
UPDATE accounts
SET provider_team_or_guild_id = $2,
    is_kiln_master            = $3,
    updated_at                = now()
WHERE id = $1;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserAccounts :many
SELECT * FROM accounts WHERE user_id = $1 ORDER BY id;

-- name: UserIsKilnMaster :one
SELECT EXISTS (SELECT 1 FROM accounts WHERE user_id = $1 AND is_kiln_master)::boolean;

-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, expires_at) VALUES ($1, $2, $3);

-- name: GetSessionUser :one
SELECT u.* FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.id = $1 AND s.expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= now();

-- name: CreateOAuthState :exec
INSERT INTO oauth_states (state, provider, return_to, browser_hash, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: ConsumeOAuthState :one
-- Single use: the row is deleted as it is read.
DELETE FROM oauth_states
WHERE state = $1 AND provider = $2 AND expires_at > now()
RETURNING *;

-- name: DeleteExpiredOAuthStates :exec
DELETE FROM oauth_states WHERE expires_at <= now();

-- name: DeleteUserSessions :execrows
-- Signs a user out everywhere (admin action, or membership lost).
DELETE FROM sessions WHERE user_id = $1;

-- name: ListAccountsToRecheck :many
-- Accounts whose membership still matters: anyone with a live session, plus
-- kiln masters (they get alerts even when signed out).
SELECT a.* FROM accounts a
WHERE a.is_kiln_master
   OR EXISTS (SELECT 1 FROM sessions s WHERE s.user_id = a.user_id AND s.expires_at > now())
ORDER BY a.id;

-- name: SetAccountKilnMaster :exec
UPDATE accounts SET is_kiln_master = $2, updated_at = now() WHERE id = $1;
