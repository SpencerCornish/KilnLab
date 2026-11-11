-- name: GetSettings :one
SELECT * FROM settings WHERE id = 1;


-- name: ListKilns :many
-- Active kilns first; inactive ones are kept so old firings still show their kiln.
SELECT * FROM kilns ORDER BY active DESC, id;

-- name: GetKiln :one
SELECT * FROM kilns WHERE id = $1;

-- name: UpdateSettings :one
UPDATE settings
SET timezone = sqlc.arg(timezone),
    min_notice_hours = sqlc.arg(min_notice_hours),
    request_cutoff_hours = sqlc.arg(request_cutoff_hours),
    underfilled_threshold_pct = sqlc.arg(underfilled_threshold_pct),
    firing_types = sqlc.arg(firing_types),
    cones = sqlc.arg(cones),
    programs = sqlc.arg(programs),
    discord_channel_id = sqlc.arg(discord_channel_id),
    slack_channel_id = sqlc.arg(slack_channel_id),
    missed_start_grace_minutes = sqlc.arg(missed_start_grace_minutes),
    missed_unload_grace_minutes = sqlc.arg(missed_unload_grace_minutes),
    public_page_days_ahead = sqlc.arg(public_page_days_ahead),
    updated_at = now()
WHERE id = 1
RETURNING *;

-- name: CreateKiln :one
INSERT INTO kilns (name, interior_width_in, interior_depth_in, interior_height_in, shelf_count, share_per_shelf_pct, active)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdateKiln :one
UPDATE kilns
SET name = sqlc.arg(name),
    interior_width_in = sqlc.narg(interior_width_in),
    interior_depth_in = sqlc.narg(interior_depth_in),
    interior_height_in = sqlc.narg(interior_height_in),
    shelf_count = sqlc.narg(shelf_count),
    share_per_shelf_pct = sqlc.narg(share_per_shelf_pct),
    active = sqlc.arg(active),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: CountActiveKilns :one
SELECT count(*)::int FROM kilns WHERE active;
