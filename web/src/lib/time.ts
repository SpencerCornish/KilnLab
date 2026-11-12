import { TZDate } from '@date-fns/tz'
import { addDays, format, formatDistanceToNowStrict } from 'date-fns'

// All times are shown in the studio's timezone (settings.timezone), not the
// phone's, so everyone sees the same wall-clock time.

export function formatWhen(iso: string, tz: string): string {
  return format(new TZDate(iso, tz), 'EEE MMM d · h:mm a')
}

// ── Scheduled days ─────────────────────────────────────────────────────────
// A firing is booked for a calendar day ("2026-11-20"), not a start time: the
// kiln runs sometime that day. Day values are plain dates, so they never shift
// with the viewer's timezone.

/** A scheduled day as a local calendar date (no timezone conversion). */
export function parseDay(ymd: string): Date {
  const [y, m, d] = ymd.split('-').map(Number)
  return new Date(y!, m! - 1, d!)
}

/** "Fri Nov 20" */
export function formatDate(ymd: string): string {
  return format(parseDay(ymd), 'EEE MMM d')
}

export function dayParts(ymd: string): { weekday: string; day: string; month: string } {
  const d = parseDay(ymd)
  return { weekday: format(d, 'EEE'), day: format(d, 'd'), month: format(d, 'MMM') }
}

/**
 * The first studio-local day (yyyy-MM-dd) whose start is at least `hours` from
 * now: the earliest a member can book.
 */
export function earliestDay(tz: string, hours: number): string {
  const earliestStart = Date.now() + hours * 3_600_000
  const today = TZDate.tz(tz)
  for (let i = 0; i < 400; i++) {
    const d = addDays(today, i)
    const start = new TZDate(d.getFullYear(), d.getMonth(), d.getDate(), 0, 0, tz)
    if (start.getTime() >= earliestStart) return format(d, 'yyyy-MM-dd')
  }
  return format(today, 'yyyy-MM-dd')
}

export function relative(iso: string): string {
  return formatDistanceToNowStrict(new Date(iso), { addSuffix: true })
}

/** ISO → value for <input type="datetime-local">, in the studio timezone. */
export function toInputValue(iso: string, tz: string): string {
  return format(new TZDate(iso, tz), "yyyy-MM-dd'T'HH:mm")
}

/** <input type="datetime-local"> value (studio wall time) → UTC ISO string. */
export function fromInputValue(value: string, tz: string): string | null {
  const m = value.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/)
  if (!m) return null
  const [, y, mo, d, h, mi] = m.map(Number)
  return new Date(new TZDate(y!, mo! - 1, d!, h!, mi!, tz).getTime()).toISOString()
}
