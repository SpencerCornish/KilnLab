-- Restores the columns empty: nobody is assigned as a loader and nobody is a self-loader.
ALTER TABLE users ADD COLUMN is_self_loader BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE firings ADD COLUMN loader_user_id BIGINT REFERENCES users (id);
