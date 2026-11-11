-- ── Outbox ──────────────────────────────────────────────────────────────────

-- name: EnqueueOutbox :exec
INSERT INTO notification_outbox (kind, platform, payload) VALUES ($1, $2, $3);

-- name: ClaimDueOutbox :one
-- One due row, locked for this transaction; other workers skip it.
SELECT * FROM notification_outbox
WHERE status = 'pending' AND next_attempt_at <= sqlc.arg(now)
ORDER BY next_attempt_at, id
LIMIT 1
FOR UPDATE SKIP LOCKED;

-- name: MarkOutboxSent :exec
UPDATE notification_outbox SET status = 'sent', attempts = attempts + 1, last_error = NULL WHERE id = $1;

-- name: MarkOutboxRetry :exec
UPDATE notification_outbox
SET attempts = attempts + 1, last_error = sqlc.arg(last_error), next_attempt_at = sqlc.arg(next_attempt_at)
WHERE id = sqlc.arg(id);

-- name: MarkOutboxFailed :exec
UPDATE notification_outbox SET status = 'failed', attempts = attempts + 1, last_error = sqlc.arg(last_error)
WHERE id = sqlc.arg(id);

-- name: ListOutbox :many
SELECT * FROM notification_outbox ORDER BY id DESC LIMIT sqlc.arg(max_rows);

-- ── Channel messages (edit in place) ────────────────────────────────────────

-- name: GetNotificationMessage :one
SELECT * FROM notification_messages WHERE firing_id = $1 AND platform = $2;

-- name: UpsertNotificationMessage :exec
INSERT INTO notification_messages (firing_id, platform, channel_id, message_id, thread_id, last_rendered_hash)
VALUES (sqlc.arg(firing_id), sqlc.arg(platform), sqlc.arg(channel_id), sqlc.arg(message_id), sqlc.narg(thread_id), sqlc.arg(last_rendered_hash))
ON CONFLICT (firing_id, platform) DO UPDATE
SET channel_id = EXCLUDED.channel_id,
    message_id = EXCLUDED.message_id,
    thread_id = EXCLUDED.thread_id,
    last_rendered_hash = EXCLUDED.last_rendered_hash,
    updated_at = now();

-- ── Recipients ──────────────────────────────────────────────────────────────

-- name: ListParticipants :many
-- The proposer plus requesters whose request is in one of the given statuses.
SELECT f.proposed_by_user_id AS user_id FROM firings f WHERE f.id = sqlc.arg(firing_id)
UNION
SELECT r.user_id FROM firing_requests r
WHERE r.firing_id = sqlc.arg(firing_id) AND r.status::text = ANY (sqlc.arg(statuses)::text[]);

-- name: ListKilnMasterIDs :many
-- Kiln-master alerts go to everyone with the Discord role (open question 3: no "on duty" rota).
SELECT DISTINCT a.user_id FROM accounts a WHERE a.is_kiln_master;

-- name: CountApprovedRequests :one
SELECT count(*)::int FROM firing_requests WHERE firing_id = $1 AND status = 'approved';
