DROP INDEX IF EXISTS firing_requests_one_live_per_user_idx;
ALTER TABLE firing_requests DROP COLUMN IF EXISTS glazed;
