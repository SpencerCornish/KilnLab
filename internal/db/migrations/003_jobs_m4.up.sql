-- M4 scheduled jobs. Set when the "requests closed" job has handled the
-- firing's cutoff (channel refresh + kiln-master alert); cleared when the
-- firing is rescheduled or its requests window changes, so it runs again.
ALTER TABLE firings ADD COLUMN requests_close_handled_at TIMESTAMPTZ;

-- Firings already past their cutoff when this ships shouldn't all alert at once.
UPDATE firings SET requests_close_handled_at = now()
WHERE requests_close_at <= now() OR requests_closed_manually;

-- Also skip reminders for anything already overdue.
UPDATE firings SET missed_start_reminder_sent_at = now()
WHERE missed_start_reminder_sent_at IS NULL AND scheduled_start <= now()
  AND status IN ('confirmed', 'loading', 'loaded');
UPDATE firings SET missed_unload_reminder_sent_at = now()
WHERE missed_unload_reminder_sent_at IS NULL AND estimated_unload_at <= now()
  AND status IN ('firing', 'cooling');
