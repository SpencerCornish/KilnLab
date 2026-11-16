-- ── Public views (/upcoming, /calendar.ics) ─────────────────────────────────

-- name: ListPublicFirings :many
-- Firings with the given statuses starting before `until`. Firings not yet
-- started must also start after `since`; in-progress ones always show.
SELECT sqlc.embed(f),
       COALESCE((SELECT sum(r.estimated_share_pct) FROM firing_requests r
                 WHERE r.firing_id = f.id AND r.status = 'approved'), 0)::int AS approved_share_pct,
       (SELECT count(*) FROM firing_requests r
         WHERE r.firing_id = f.id AND r.status = 'approved')::int AS approved_count
FROM firings f
WHERE f.status::text = ANY (sqlc.arg(statuses)::text[])
  AND f.scheduled_date <= sqlc.arg(until)
  AND (f.scheduled_date >= sqlc.arg(since)
       OR f.status IN ('firing', 'cooling'))
ORDER BY f.scheduled_date, f.id;
