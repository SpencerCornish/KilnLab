-- Restores scheduled_start as midnight at the start of each firing's day in
-- the studio timezone. The original start times aren't recoverable.
ALTER TABLE firings ADD COLUMN scheduled_start TIMESTAMPTZ;

UPDATE firings
SET scheduled_start = scheduled_date::timestamp AT TIME ZONE COALESCE(
    (SELECT timezone FROM settings WHERE id = 1), 'America/Denver');

ALTER TABLE firings ALTER COLUMN scheduled_start SET NOT NULL;
DROP INDEX firings_scheduled_date_idx;
ALTER TABLE firings DROP COLUMN scheduled_date;
CREATE INDEX firings_scheduled_start_idx ON firings (scheduled_start);
