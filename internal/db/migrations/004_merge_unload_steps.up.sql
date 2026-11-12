-- Unloading, ready for pickup and completed are one step now: marking a firing
-- completed unloads it and tells participants it's ready. Firings still in the
-- old unloaded / ready_for_pickup steps move to completed, and their times are
-- folded into one unloaded time (ready_for_pickup_at and completed_at mirror it).

UPDATE firings
SET unloaded_at = COALESCE(unloaded_at, ready_for_pickup_at, completed_at, updated_at),
    ready_for_pickup_at = COALESCE(unloaded_at, ready_for_pickup_at, completed_at, updated_at),
    completed_at = COALESCE(unloaded_at, ready_for_pickup_at, completed_at, updated_at)
WHERE status IN ('unloaded', 'ready_for_pickup');

ALTER TABLE firings ALTER COLUMN status DROP DEFAULT;
ALTER TABLE firing_events ALTER COLUMN from_status TYPE TEXT USING from_status::text;
ALTER TABLE firing_events ALTER COLUMN to_status TYPE TEXT USING to_status::text;

ALTER TYPE firing_status RENAME TO firing_status_old;
CREATE TYPE firing_status AS ENUM (
    'proposed', 'on_hold', 'confirmed', 'loading', 'loaded',
    'firing', 'cooling', 'completed', 'cancelled'
);

UPDATE firing_events SET from_status = 'completed' WHERE from_status IN ('unloaded', 'ready_for_pickup');
UPDATE firing_events SET to_status = 'completed' WHERE to_status IN ('unloaded', 'ready_for_pickup');

ALTER TABLE firings ALTER COLUMN status TYPE firing_status USING (
    CASE WHEN status::text IN ('unloaded', 'ready_for_pickup') THEN 'completed' ELSE status::text END
)::firing_status;
ALTER TABLE firing_events ALTER COLUMN from_status TYPE firing_status USING from_status::firing_status;
ALTER TABLE firing_events ALTER COLUMN to_status TYPE firing_status USING to_status::firing_status;
ALTER TABLE firings ALTER COLUMN status SET DEFAULT 'proposed';

DROP TYPE firing_status_old;
