-- M1: requests carry a "glazed work" flag for the bisque compatibility check (§7),
-- and a user can have at most one live (pending/approved) request per firing.
ALTER TABLE firing_requests ADD COLUMN glazed BOOLEAN NOT NULL DEFAULT false;

CREATE UNIQUE INDEX firing_requests_one_live_per_user_idx
    ON firing_requests (firing_id, user_id)
    WHERE status IN ('pending', 'approved');
