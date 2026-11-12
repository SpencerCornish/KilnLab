-- Loaders are gone: anyone signed in can say they've loaded or unloaded work,
-- so there's no assigned loader and no self-loader flag.
ALTER TABLE firings DROP COLUMN loader_user_id;
ALTER TABLE users DROP COLUMN is_self_loader;
