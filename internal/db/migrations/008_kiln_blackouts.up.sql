-- Admin-only kiln date blackouts: maintenance and other reasons a kiln can't
-- take new firings. start_date/end_date are inclusive studio calendar days.
-- Blackouts gate new proposals and reschedules; they never cancel an existing
-- firing. Every kiln can be blacked out independently.
CREATE TABLE kiln_blackouts (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kiln_id    BIGINT      NOT NULL REFERENCES kilns (id) ON DELETE CASCADE,
    start_date DATE        NOT NULL,
    end_date   DATE        NOT NULL,
    reason     TEXT        NOT NULL DEFAULT '',
    created_by BIGINT      REFERENCES users (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (end_date >= start_date)
);
CREATE INDEX kiln_blackouts_kiln_date_idx ON kiln_blackouts (kiln_id, start_date, end_date);
