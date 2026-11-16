-- name: GetSettings :one
SELECT * FROM settings WHERE id = 1;

-- name: UpdateSettings :one
UPDATE settings
SET timezone = sqlc.arg(timezone),
    min_notice_hours = sqlc.arg(min_notice_hours),
    request_cutoff_hours = sqlc.arg(request_cutoff_hours),
    underfilled_threshold_pct = sqlc.arg(underfilled_threshold_pct),
    firing_types = sqlc.arg(firing_types),
    cones = sqlc.arg(cones),
    discord_channel_id = sqlc.arg(discord_channel_id),
    slack_channel_id = sqlc.arg(slack_channel_id),
    missed_start_grace_minutes = sqlc.arg(missed_start_grace_minutes),
    missed_unload_grace_minutes = sqlc.arg(missed_unload_grace_minutes),
    escalation_grace_minutes = sqlc.arg(escalation_grace_minutes),
    day_reminder_hour = sqlc.arg(day_reminder_hour),
    public_page_days_ahead = sqlc.arg(public_page_days_ahead),
    updated_at = now()
WHERE id = 1
RETURNING *;

-- ── Kilns ───────────────────────────────────────────────────────────────────

-- name: ListKilns :many
-- Active kilns first; inactive ones are kept so old firings still show their kiln.
SELECT * FROM kilns ORDER BY active DESC, id;

-- name: GetKiln :one
SELECT * FROM kilns WHERE id = $1;

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

-- ── Firing profiles ─────────────────────────────────────────────────────────

-- name: ListProfiles :many
-- Every profile, active or not, for the admin editor.
SELECT * FROM firing_profiles ORDER BY kiln_id, lower(name), id;

-- name: ListActiveProfiles :many
-- Profiles a proposer can choose from, grouped by kiln.
SELECT * FROM firing_profiles WHERE active ORDER BY kiln_id, lower(name), id;

-- name: GetProfile :one
SELECT * FROM firing_profiles WHERE id = $1;

-- name: CreateProfile :one
INSERT INTO firing_profiles (kiln_id, name, firing_type, cone, firing_minutes, cooldown_minutes, active)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdateProfile :one
UPDATE firing_profiles
SET kiln_id = sqlc.arg(kiln_id),
    name = sqlc.arg(name),
    firing_type = sqlc.arg(firing_type),
    cone = sqlc.arg(cone),
    firing_minutes = sqlc.arg(firing_minutes),
    cooldown_minutes = sqlc.arg(cooldown_minutes),
    active = sqlc.arg(active),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeactivateProfile :one
UPDATE firing_profiles SET active = false, updated_at = now() WHERE id = $1 RETURNING *;
