-- KilnLab initial schema (v1). All timestamps are UTC (timestamptz); display in settings.timezone.

CREATE TYPE community AS ENUM ('artlab', 'bms');
CREATE TYPE auth_provider AS ENUM ('discord', 'slack');

CREATE TYPE firing_status AS ENUM (
    'proposed', 'on_hold', 'confirmed', 'loading', 'loaded',
    'firing', 'cooling', 'unloaded', 'ready_for_pickup', 'completed', 'cancelled'
);

CREATE TYPE request_status AS ENUM ('pending', 'approved', 'declined', 'withdrawn');
CREATE TYPE event_source AS ENUM ('web', 'kiln_qr', 'system');
CREATE TYPE outbox_kind AS ENUM ('channel_upsert', 'dm');
CREATE TYPE outbox_status AS ENUM ('pending', 'sent', 'failed');

-- ── Identity ────────────────────────────────────────────────────────────────

-- users is separate from accounts so account linking (same person on Discord
-- and Slack) can be added later without a data migration.
CREATE TABLE users (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    display_name   TEXT        NOT NULL,
    avatar_url     TEXT,
    community      community   NOT NULL,
    is_self_loader BOOLEAN     NOT NULL DEFAULT false,
    is_admin       BOOLEAN     NOT NULL DEFAULT false,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE accounts (
    id                        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id                   BIGINT        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider                  auth_provider NOT NULL,
    provider_user_id          TEXT          NOT NULL,
    provider_team_or_guild_id TEXT          NOT NULL,
    -- Refreshed at every login from the Discord guild roles. Always false for Slack.
    is_kiln_master            BOOLEAN       NOT NULL DEFAULT false,
    created_at                TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_user_id)
);
CREATE INDEX accounts_user_id_idx ON accounts (user_id);
CREATE INDEX accounts_kiln_master_idx ON accounts (user_id) WHERE is_kiln_master;

CREATE TABLE sessions (
    id         TEXT        PRIMARY KEY, -- SHA-256 of the cookie token, hex
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

-- OAuth state is kept server-side (not only in a cookie) because on phones the
-- OAuth round trip can bounce through the Discord/Slack app and come back in a
-- different browser that never saw our cookie.
CREATE TABLE oauth_states (
    state        TEXT          PRIMARY KEY,
    provider     auth_provider NOT NULL,
    return_to    TEXT          NOT NULL,
    browser_hash TEXT          NOT NULL, -- SHA-256 of the per-browser binding cookie
    expires_at   TIMESTAMPTZ   NOT NULL,
    created_at   TIMESTAMPTZ   NOT NULL DEFAULT now()
);

-- ── Kilns & settings ────────────────────────────────────────────────────────

CREATE TABLE kilns (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name                TEXT        NOT NULL,
    interior_width_in   DOUBLE PRECISION,
    interior_depth_in   DOUBLE PRECISION,
    interior_height_in  DOUBLE PRECISION,
    shelf_count         INTEGER,
    share_per_shelf_pct INTEGER CHECK (share_per_shelf_pct BETWEEN 1 AND 100),
    active              BOOLEAN     NOT NULL DEFAULT true,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO kilns (name) VALUES ('Studio kiln');

CREATE TABLE settings (
    id                          INTEGER     PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    timezone                    TEXT        NOT NULL DEFAULT 'America/Denver',
    min_notice_hours            INTEGER     NOT NULL DEFAULT 48 CHECK (min_notice_hours >= 0),
    request_cutoff_hours        INTEGER     NOT NULL DEFAULT 12 CHECK (request_cutoff_hours >= 0),
    underfilled_threshold_pct   INTEGER     NOT NULL DEFAULT 50 CHECK (underfilled_threshold_pct BETWEEN 0 AND 100),
    firing_types                TEXT[]      NOT NULL DEFAULT ARRAY['bisque', 'glaze', 'other'],
    cones                       TEXT[]      NOT NULL DEFAULT ARRAY['022', '018', '010', '06', '04', '5', '6', '10'],
    -- [{"name": "Slow Glaze", "default_duration_minutes": 600}, ...]
    programs                    JSONB       NOT NULL DEFAULT '[]'::jsonb,
    discord_channel_id          TEXT        NOT NULL DEFAULT '',
    slack_channel_id            TEXT        NOT NULL DEFAULT '',
    missed_start_grace_minutes  INTEGER     NOT NULL DEFAULT 120 CHECK (missed_start_grace_minutes >= 0),
    missed_unload_grace_minutes INTEGER     NOT NULL DEFAULT 180 CHECK (missed_unload_grace_minutes >= 0),
    public_page_days_ahead      INTEGER     NOT NULL DEFAULT 30 CHECK (public_page_days_ahead > 0),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO settings (id) VALUES (1);

-- ── Firings ─────────────────────────────────────────────────────────────────

CREATE TABLE firings (
    id                             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kiln_id                        BIGINT        NOT NULL REFERENCES kilns (id),
    proposed_by_user_id            BIGINT        NOT NULL REFERENCES users (id),
    firing_type                    TEXT          NOT NULL,
    cone                           TEXT          NOT NULL,
    title                          TEXT,
    notes                          TEXT          NOT NULL DEFAULT '',
    scheduled_start                TIMESTAMPTZ   NOT NULL,
    status                         firing_status NOT NULL DEFAULT 'proposed',
    -- Set when a firing is held, so release can return to confirmed if it had been.
    was_confirmed                  BOOLEAN       NOT NULL DEFAULT false,
    hold_reason                    TEXT,
    cancel_reason                  TEXT,
    proposer_description           TEXT          NOT NULL,
    proposer_share_pct             INTEGER       NOT NULL CHECK (proposer_share_pct BETWEEN 1 AND 100),
    proposer_cone                  TEXT          NOT NULL,
    proposer_clay_body             TEXT,
    fullness_override_pct          INTEGER CHECK (fullness_override_pct BETWEEN 0 AND 100),
    requests_close_at              TIMESTAMPTZ   NOT NULL,
    requests_closed_manually       BOOLEAN       NOT NULL DEFAULT false,
    loader_user_id                 BIGINT REFERENCES users (id),
    program                        TEXT,
    estimated_duration_minutes     INTEGER CHECK (estimated_duration_minutes > 0),
    started_at                     TIMESTAMPTZ,
    estimated_unload_at            TIMESTAMPTZ,
    cooling_at                     TIMESTAMPTZ,
    unloaded_at                    TIMESTAMPTZ,
    ready_for_pickup_at            TIMESTAMPTZ,
    completed_at                   TIMESTAMPTZ,
    underfilled_alert_sent_at      TIMESTAMPTZ,
    missed_start_reminder_sent_at  TIMESTAMPTZ,
    missed_unload_reminder_sent_at TIMESTAMPTZ,
    created_at                     TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at                     TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX firings_scheduled_start_idx ON firings (scheduled_start);
CREATE INDEX firings_status_idx ON firings (status);

CREATE TABLE firing_requests (
    id                    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    firing_id             BIGINT         NOT NULL REFERENCES firings (id) ON DELETE CASCADE,
    user_id               BIGINT         NOT NULL REFERENCES users (id),
    description           TEXT           NOT NULL,
    estimated_share_pct   INTEGER        NOT NULL CHECK (estimated_share_pct BETWEEN 1 AND 100),
    cone                  TEXT           NOT NULL,
    clay_body             TEXT,
    status                request_status NOT NULL DEFAULT 'pending',
    decision_note         TEXT,
    decided_by_user_id    BIGINT REFERENCES users (id),
    decided_at            TIMESTAMPTZ,
    compatibility_warning TEXT,
    created_at            TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ    NOT NULL DEFAULT now()
);
CREATE INDEX firing_requests_firing_id_idx ON firing_requests (firing_id);
CREATE INDEX firing_requests_user_id_idx ON firing_requests (user_id);

CREATE TABLE firing_events (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    firing_id     BIGINT        NOT NULL REFERENCES firings (id) ON DELETE CASCADE,
    actor_user_id BIGINT REFERENCES users (id), -- NULL for system events
    type          TEXT          NOT NULL,
    from_status   firing_status,
    to_status     firing_status,
    occurred_at   TIMESTAMPTZ   NOT NULL DEFAULT now(), -- may be back-dated
    recorded_at   TIMESTAMPTZ   NOT NULL DEFAULT now(),
    note          TEXT,
    source        event_source  NOT NULL DEFAULT 'web'
);
CREATE INDEX firing_events_firing_id_idx ON firing_events (firing_id, occurred_at);

-- ── Notifications ───────────────────────────────────────────────────────────

CREATE TABLE notification_messages (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    firing_id          BIGINT        NOT NULL REFERENCES firings (id) ON DELETE CASCADE,
    platform           auth_provider NOT NULL,
    channel_id         TEXT          NOT NULL,
    message_id         TEXT          NOT NULL, -- Discord message id / Slack ts
    thread_id          TEXT,                   -- Discord thread channel id (Slack threads use message_id)
    last_rendered_hash TEXT          NOT NULL DEFAULT '',
    updated_at         TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (firing_id, platform)
);

CREATE TABLE notification_outbox (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind            outbox_kind   NOT NULL,
    platform        auth_provider NOT NULL,
    payload         JSONB         NOT NULL,
    status          outbox_status NOT NULL DEFAULT 'pending',
    attempts        INTEGER       NOT NULL DEFAULT 0,
    last_error      TEXT,
    next_attempt_at TIMESTAMPTZ   NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX notification_outbox_due_idx ON notification_outbox (next_attempt_at) WHERE status = 'pending';
