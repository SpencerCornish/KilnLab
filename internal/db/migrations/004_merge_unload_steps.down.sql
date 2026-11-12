-- Restores the enum values. Completed firings stay completed: the separate
-- unloaded / ready_for_pickup steps can't be reconstructed from the merged data.

ALTER TABLE firings ALTER COLUMN status DROP DEFAULT;
ALTER TABLE firing_events ALTER COLUMN from_status TYPE TEXT USING from_status::text;
ALTER TABLE firing_events ALTER COLUMN to_status TYPE TEXT USING to_status::text;

ALTER TYPE firing_status RENAME TO firing_status_new;
CREATE TYPE firing_status AS ENUM (
    'proposed', 'on_hold', 'confirmed', 'loading', 'loaded',
    'firing', 'cooling', 'unloaded', 'ready_for_pickup', 'completed', 'cancelled'
);

ALTER TABLE firings ALTER COLUMN status TYPE firing_status USING status::text::firing_status;
ALTER TABLE firing_events ALTER COLUMN from_status TYPE firing_status USING from_status::firing_status;
ALTER TABLE firing_events ALTER COLUMN to_status TYPE firing_status USING to_status::firing_status;
ALTER TABLE firings ALTER COLUMN status SET DEFAULT 'proposed';

DROP TYPE firing_status_new;
