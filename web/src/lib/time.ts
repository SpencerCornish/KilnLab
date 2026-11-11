import { TZDate } from '@date-fns/tz'
import { format, formatDistanceToNowStrict } from 'date-fns'

// All times are shown in the studio's timezone (settings.timezone), not the
// phone's, so everyone sees the same wall-clock time.

export function formatWhen(iso: string, tz: string): string {
  return format(new TZDate(iso, tz), 'EEE MMM d · h:mm a')
}

export function formatDay(iso: string, tz: string): { weekday: string; day: string; month: string; time: string } {
  const d = new TZDate(iso, tz)
  return { weekday: format(d, 'EEE'), day: format(d, 'd'), month: format(d, 'MMM'), time: format(d, 'h:mm a') }
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
