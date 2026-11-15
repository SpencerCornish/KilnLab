-- A kiln-master reminder on the morning of each firing's day. The hour is
-- studio time (settings.timezone), admin-editable; 8am by default.
ALTER TABLE settings ADD COLUMN day_reminder_hour INTEGER NOT NULL DEFAULT 8
    CHECK (day_reminder_hour BETWEEN 0 AND 23);

-- Set once the "firing is today" reminder has been sent, so it fires exactly once.
ALTER TABLE firings ADD COLUMN day_reminder_sent_at TIMESTAMPTZ;

-- Firings already past their day (or cancelled) shouldn't all remind at once on
-- the first tick after deploying. Today's and future non-cancelled firings keep
-- a NULL marker so they still get their reminder.
UPDATE firings SET day_reminder_sent_at = now()
WHERE day_reminder_sent_at IS NULL
  AND (status = 'cancelled'
       OR scheduled_date < ((now() AT TIME ZONE COALESCE(
           (SELECT timezone FROM settings WHERE id = 1), 'America/Denver'))::date));
