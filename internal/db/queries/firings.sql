-- ── Firings ─────────────────────────────────────────────────────────────────

-- name: CreateFiring :one
INSERT INTO firings (
    kiln_id, proposed_by_user_id, firing_type, cone, title, notes, scheduled_start,
    proposer_description, proposer_share_pct, proposer_cone, proposer_clay_body, requests_close_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
RETURNING *;

-- name: GetFiring :one
SELECT * FROM firings WHERE id = $1;

-- name: GetFiringForUpdate :one
SELECT * FROM firings WHERE id = $1 FOR UPDATE;

-- name: ListFiringSummaries :many
-- Active firings, plus finished/cancelled ones scheduled after `since`.
SELECT sqlc.embed(f),
       u.display_name AS proposer_name,
       COALESCE((SELECT sum(r.estimated_share_pct) FROM firing_requests r
                 WHERE r.firing_id = f.id AND r.status = 'approved'), 0)::int AS approved_share_pct,
       (SELECT count(*) FROM firing_requests r
         WHERE r.firing_id = f.id AND r.status = 'approved')::int AS approved_count,
       (SELECT count(*) FROM firing_requests r
         WHERE r.firing_id = f.id AND r.status = 'pending')::int AS pending_count,
       EXISTS (SELECT 1 FROM firing_requests r
               WHERE r.firing_id = f.id AND r.user_id = sqlc.arg(user_id)
                 AND r.status IN ('pending', 'approved')) AS has_my_request
FROM firings f
JOIN users u ON u.id = f.proposed_by_user_id
WHERE f.status NOT IN ('completed', 'cancelled') OR f.scheduled_start >= sqlc.arg(since)
ORDER BY f.scheduled_start, f.id;

-- name: SetFiringStatus :one
UPDATE firings
SET status = sqlc.arg(status),
    was_confirmed = sqlc.arg(was_confirmed),
    hold_reason = sqlc.narg(hold_reason),
    cancel_reason = sqlc.narg(cancel_reason),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: RescheduleFiring :one
-- Clears job idempotency markers so reminders fire again for the new time.
UPDATE firings
SET scheduled_start = sqlc.arg(scheduled_start),
    requests_close_at = sqlc.arg(requests_close_at),
    underfilled_alert_sent_at = NULL,
    missed_start_reminder_sent_at = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetFullnessOverride :one
UPDATE firings SET fullness_override_pct = sqlc.narg(override_pct), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetRequestsWindow :one
UPDATE firings
SET requests_closed_manually = sqlc.arg(closed_manually),
    requests_close_at = sqlc.arg(requests_close_at),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetLoader :one
UPDATE firings SET loader_user_id = sqlc.narg(loader_user_id), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- ── Requests ────────────────────────────────────────────────────────────────

-- name: ListRequestsForFiring :many
SELECT sqlc.embed(r), u.display_name AS user_name, u.avatar_url AS user_avatar_url, u.community AS user_community
FROM firing_requests r
JOIN users u ON u.id = r.user_id
WHERE r.firing_id = $1
ORDER BY r.created_at, r.id;

-- name: ApprovedShares :many
SELECT estimated_share_pct FROM firing_requests
WHERE firing_id = $1 AND status = 'approved';

-- name: GetRequestForUpdate :one
SELECT * FROM firing_requests WHERE id = $1 FOR UPDATE;

-- name: UserHasLiveRequest :one
SELECT EXISTS (
    SELECT 1 FROM firing_requests
    WHERE firing_id = $1 AND user_id = $2 AND status IN ('pending', 'approved')
);

-- name: CreateRequest :one
INSERT INTO firing_requests (
    firing_id, user_id, description, estimated_share_pct, cone, clay_body, glazed, compatibility_warning
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpdateRequest :one
-- Editing an approved request sends it back to pending for the kiln master.
UPDATE firing_requests
SET description = sqlc.arg(description),
    estimated_share_pct = sqlc.arg(estimated_share_pct),
    cone = sqlc.arg(cone),
    clay_body = sqlc.narg(clay_body),
    glazed = sqlc.arg(glazed),
    compatibility_warning = sqlc.narg(compatibility_warning),
    status = 'pending',
    decision_note = NULL,
    decided_by_user_id = NULL,
    decided_at = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: WithdrawRequest :one
UPDATE firing_requests SET status = 'withdrawn', updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DecideRequest :one
UPDATE firing_requests
SET status = sqlc.arg(status),
    decision_note = sqlc.narg(decision_note),
    decided_by_user_id = sqlc.arg(decided_by_user_id),
    decided_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- ── Events ──────────────────────────────────────────────────────────────────

-- name: InsertEvent :exec
INSERT INTO firing_events (firing_id, actor_user_id, type, from_status, to_status, occurred_at, note, source)
VALUES (
    sqlc.arg(firing_id), sqlc.narg(actor_user_id), sqlc.arg(type), sqlc.narg(from_status),
    sqlc.narg(to_status), sqlc.arg(occurred_at), sqlc.narg(note), sqlc.arg(source)
);

-- name: ListEventsForFiring :many
SELECT e.*, u.display_name AS actor_name
FROM firing_events e
LEFT JOIN users u ON u.id = e.actor_user_id
WHERE e.firing_id = $1
ORDER BY e.occurred_at, e.id;

-- ── Users ───────────────────────────────────────────────────────────────────

-- name: ListLoaderCandidates :many
-- Self-loaders and kiln masters (admins are offered too).
SELECT u.id, u.display_name, u.is_self_loader,
       EXISTS (SELECT 1 FROM accounts a WHERE a.user_id = u.id AND a.is_kiln_master) AS is_kiln_master
FROM users u
WHERE u.is_self_loader OR u.is_admin
   OR EXISTS (SELECT 1 FROM accounts a WHERE a.user_id = u.id AND a.is_kiln_master)
ORDER BY u.display_name;

-- name: GetUserName :one
SELECT display_name FROM users WHERE id = $1;

-- ── Lifecycle (M2) ──────────────────────────────────────────────────────────

-- name: AdvanceFiring :one
-- Moves along the lifecycle. NULL arguments leave columns unchanged.
UPDATE firings
SET status = sqlc.arg(status),
    program = COALESCE(sqlc.narg(program), program),
    estimated_duration_minutes = COALESCE(sqlc.narg(estimated_duration_minutes), estimated_duration_minutes),
    started_at = COALESCE(sqlc.narg(started_at), started_at),
    estimated_unload_at = COALESCE(sqlc.narg(estimated_unload_at), estimated_unload_at),
    cooling_at = COALESCE(sqlc.narg(cooling_at), cooling_at),
    unloaded_at = COALESCE(sqlc.narg(unloaded_at), unloaded_at),
    ready_for_pickup_at = COALESCE(sqlc.narg(ready_for_pickup_at), ready_for_pickup_at),
    completed_at = COALESCE(sqlc.narg(completed_at), completed_at),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: CorrectTimestamps :one
-- Sets all lifecycle timestamps at once (the caller changes one and validates order).
UPDATE firings
SET started_at = sqlc.narg(started_at),
    estimated_unload_at = sqlc.narg(estimated_unload_at),
    cooling_at = sqlc.narg(cooling_at),
    unloaded_at = sqlc.narg(unloaded_at),
    ready_for_pickup_at = sqlc.narg(ready_for_pickup_at),
    completed_at = sqlc.narg(completed_at),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ListKilnCandidates :many
-- Firings that might be relevant at the kiln; firing.RelevantAtKiln makes the final call.
-- kiln_id 0 means every kiln.
SELECT sqlc.embed(f), u.display_name AS proposer_name
FROM firings f
JOIN users u ON u.id = f.proposed_by_user_id
WHERE (sqlc.arg(kiln_id)::bigint = 0 OR f.kiln_id = sqlc.arg(kiln_id))
  AND (f.status IN ('loading', 'loaded', 'firing', 'cooling', 'unloaded')
       OR (f.status = 'confirmed' AND f.scheduled_start BETWEEN sqlc.arg(window_start) AND sqlc.arg(window_end)))
ORDER BY f.scheduled_start, f.id;
