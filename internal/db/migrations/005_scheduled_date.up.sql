-- A firing is booked for a day, not a start time: the start time is not a
-- promise. Existing start times become their studio-local calendar day.
-- Started/cooling/unloaded timestamps stay full timestamps (history).
ALTER TABLE firings ADD COLUMN scheduled_date DATE;

UPDATE firings
SET scheduled_date = (scheduled_start AT TIME ZONE COALESCE(
    (SELECT timezone FROM settings WHERE id = 1), 'America/Denver'))::date;

ALTER TABLE firings ALTER COLUMN scheduled_date SET NOT NULL;
DROP INDEX firings_scheduled_start_idx;
ALTER TABLE firings DROP COLUMN scheduled_start;
CREATE INDEX firings_scheduled_date_idx ON firings (scheduled_date);
