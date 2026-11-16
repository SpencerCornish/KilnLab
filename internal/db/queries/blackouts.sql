-- ── Kiln blackouts (admin-only availability blocks) ─────────────────────────

-- name: ListBlackouts :many
-- Every blackout, for the admin page.
SELECT * FROM kiln_blackouts ORDER BY kiln_id, start_date;

-- name: ListBlackoutsForKiln :many
-- One kiln's blackouts, for scheduling checks.
SELECT * FROM kiln_blackouts WHERE kiln_id = $1 ORDER BY start_date;

-- name: CreateBlackout :one
INSERT INTO kiln_blackouts (kiln_id, start_date, end_date, reason, created_by)
VALUES (sqlc.arg(kiln_id), sqlc.arg(start_date), sqlc.arg(end_date), sqlc.arg(reason), sqlc.narg(created_by))
RETURNING *;

-- name: DeleteBlackout :execrows
DELETE FROM kiln_blackouts WHERE id = $1;
