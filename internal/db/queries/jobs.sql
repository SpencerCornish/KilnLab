-- ── Scheduled jobs (M4, spec §10) ──────────────────────────────────────────

-- name: ListJobCandidates :many
-- Firings some job might act on. jobs.Due makes the final call (grace periods etc.).
SELECT id FROM firings
WHERE (status IN ('proposed', 'on_hold', 'confirmed') AND requests_close_handled_at IS NULL
       AND (requests_close_at <= sqlc.arg(now) OR requests_closed_manually))
   OR (status IN ('confirmed', 'loading', 'loaded') AND missed_start_reminder_sent_at IS NULL
       AND scheduled_date <= sqlc.arg(today))
   OR (status IN ('firing', 'cooling') AND missed_unload_reminder_sent_at IS NULL
       AND estimated_unload_at <= sqlc.arg(now))
   OR (status IN ('proposed', 'on_hold', 'confirmed', 'loading', 'loaded') AND day_reminder_sent_at IS NULL
       AND scheduled_date <= sqlc.arg(today))
ORDER BY id;

-- name: MarkJobsDone :one
-- NULL arguments leave a marker unchanged.
UPDATE firings
SET requests_close_handled_at = COALESCE(sqlc.narg(requests_close_handled_at), requests_close_handled_at),
    underfilled_alert_sent_at = COALESCE(sqlc.narg(underfilled_alert_sent_at), underfilled_alert_sent_at),
    missed_start_reminder_sent_at = COALESCE(sqlc.narg(missed_start_reminder_sent_at), missed_start_reminder_sent_at),
    missed_unload_reminder_sent_at = COALESCE(sqlc.narg(missed_unload_reminder_sent_at), missed_unload_reminder_sent_at),
    day_reminder_sent_at = COALESCE(sqlc.narg(day_reminder_sent_at), day_reminder_sent_at)
WHERE id = sqlc.arg(id)
RETURNING *;
