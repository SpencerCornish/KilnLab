import { useEffect, useState } from 'react'
import { Link } from 'react-router'
import { Alert, firingTitle, FullnessBar, StatusBadge } from '../components/firing'
import { getUpcoming } from '../lib/api'
import { PageSpinner, useSession } from '../lib/session'
import { dayParts, formatWhen } from '../lib/time'
import type { FiringStatus, PublicFiring } from '../lib/types'
import { useLoad } from '../lib/useLoad'

// Public wording (§9A): "on hold" reads as "waiting for more work" to visitors.
const PUBLIC_LABELS: Partial<Record<FiringStatus, string>> = { on_hold: 'Waiting for more work' }

const IN_KILN = new Set<FiringStatus>(['loading', 'loaded', 'firing', 'cooling'])

/** The wheel-QR page: no login needed. Refreshes itself every minute. */
export default function UpcomingPage() {
  const { me } = useSession()
  const { data, error, reload } = useLoad(getUpcoming, [])
  useEffect(() => {
    const t = setInterval(() => void reload(), 60_000)
    return () => clearInterval(t)
  }, [reload])

  return (
    <div className="space-y-6">
      <section className="space-y-2">
        <h1 className="kc-title">Upcoming kiln firings</h1>
        <p className="kc-caption kc-caption--body">
          Have work to fire? Request space in a firing below, or propose a new one.
          {!me && ' You’ll sign in with Discord or Slack.'}
        </p>
      </section>

      <div className="kc-keys">
        <Link to="/firings/new" className="kc-key kc-key--block kc-key--fire">
          + Propose a firing
        </Link>
        {me && (
          <Link to="/" className="kc-key kc-key--block">
            My firings
          </Link>
        )}
      </div>

      {error && <Alert kind="error">Couldn't load firings: {error}</Alert>}
      {!data && !error && <PageSpinner />}
      {data && (
        <section className="space-y-3">
          {data.firings.length === 0 ? (
            <div className="kc-plate kc-plate--pad text-center">
              <p className="kc-caption kc-caption--body">
                Nothing scheduled in the next {data.days_ahead} days. Be the first to propose a firing!
              </p>
            </div>
          ) : (
            data.firings.map((f) => <PublicFiringCard key={f.id} firing={f} tz={data.timezone} />)
          )}
        </section>
      )}

      <CalendarLink />
    </div>
  )
}

function PublicFiringCard({ firing: f, tz }: { firing: PublicFiring; tz: string }) {
  const d = dayParts(f.scheduled_date)
  const inKiln = IN_KILN.has(f.status)
  const people = `${f.participant_count} ${f.participant_count === 1 ? 'participant' : 'participants'}`
  return (
    <article className="kc-plate">
      <header className="kc-plate__head">
        <span className="kc-label kc-label--date">
          {d.weekday} {d.month} {d.day}
        </span>
        <StatusBadge status={f.status} label={PUBLIC_LABELS[f.status]} />
      </header>
      <div className="kc-plate__body kc-plate__body--tight">
        <h2 className="kc-title kc-title--sm">{firingTitle(f)}</h2>
        <p className="kc-caption kc-caption--body">
          {f.kiln_name && `${f.kiln_name} · `}
          {people}
        </p>

        {inKiln ? (
          <>
            <p className="kc-data">In the kiln now</p>
            {f.estimated_unload_at && <p className="kc-data kc-muted">Est. unload {formatWhen(f.estimated_unload_at, tz)}</p>}
          </>
        ) : (
          <>
            <FullnessBar
              fullness={{ display_pct: f.fullness_pct, room_left_pct: f.room_left_pct, calculated_pct: f.fullness_pct, overridden: false }}
            />
            <p className="kc-caption kc-caption--body">
              {f.requests_open ? `Requests close ${formatWhen(f.requests_close_at, tz)}` : 'Requests closed'}
            </p>
            {f.requests_open ? (
              <Link to={`/firings/${f.id}?request=1`} className="kc-key kc-key--block kc-key--fire">
                Request space
              </Link>
            ) : (
              <Link to={`/firings/${f.id}`} className="kc-key kc-key--block">
                Details
              </Link>
            )}
          </>
        )}
      </div>
    </article>
  )
}

function CalendarLink() {
  const [copied, setCopied] = useState(false)
  const url = `${window.location.origin}/calendar.ics`
  const webcal = url.replace(/^https?:/, 'webcal:')
  async function copy() {
    try {
      await navigator.clipboard.writeText(url)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // Clipboard blocked: the URL is shown for manual copying.
    }
  }
  return (
    <section className="kc-plate">
      <header className="kc-plate__head">
        <span className="kc-label">Kiln calendar</span>
      </header>
      <div className="kc-plate__body kc-plate__body--tight">
        <p className="kc-caption kc-caption--body">Subscribe to see upcoming firings in your calendar app.</p>
        <div className="kc-keys">
          <a href={webcal} className="kc-key kc-key--sm">
            Add to calendar
          </a>
          <button onClick={() => void copy()} className="kc-key kc-key--sm">
            {copied ? 'Copied!' : 'Copy link'}
          </button>
        </div>
        <p className="kc-data break-all">{url}</p>
      </div>
    </section>
  )
}
