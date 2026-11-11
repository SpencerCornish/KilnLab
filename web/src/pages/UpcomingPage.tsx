import { useEffect, useState } from 'react'
import { Link } from 'react-router'
import { Alert, firingTitle, FullnessBar, STATUS_LABELS } from '../components/firing'
import { getUpcoming } from '../lib/api'
import { PageSpinner, useSession } from '../lib/session'
import { formatDay, formatWhen } from '../lib/time'
import type { FiringStatus, PublicFiring } from '../lib/types'
import { useLoad } from '../lib/useLoad'

// Public wording (§9A): "on hold" reads as "waiting for more work" to visitors.
const PUBLIC_LABELS: Partial<Record<FiringStatus, string>> = { on_hold: 'Waiting for more work' }

const PUBLIC_STYLES: Partial<Record<FiringStatus, string>> = {
  proposed: 'bg-sky-100 text-sky-900',
  on_hold: 'bg-amber-100 text-amber-900',
  confirmed: 'bg-green-100 text-green-900',
}

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
      <section className="space-y-1">
        <h1 className="text-2xl font-bold">Upcoming kiln firings</h1>
        <p className="text-stone-600">
          Have work to fire? Request space in a firing below, or propose a new one.
          {!me && ' You’ll sign in with Discord or Slack.'}
        </p>
      </section>

      <div className="grid grid-cols-2 gap-3">
        <Link to="/firings/new" className="btn col-span-2 bg-orange-600 text-white">
          + Propose a firing
        </Link>
        {me && (
          <Link to="/" className="btn col-span-2 border-2 border-stone-300 bg-white text-base">
            My firings
          </Link>
        )}
      </div>

      {error && <Alert kind="error">Couldn't load firings: {error}</Alert>}
      {!data && !error && <PageSpinner />}
      {data && (
        <section className="space-y-3">
          {data.firings.length === 0 ? (
            <div className="card text-center text-stone-600">
              Nothing scheduled in the next {data.days_ahead} days. Be the first to propose a firing!
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
  const d = formatDay(f.scheduled_start, tz)
  const inKiln = IN_KILN.has(f.status)
  const people = `${f.participant_count} ${f.participant_count === 1 ? 'participant' : 'participants'}`
  return (
    <article className="card space-y-3">
      <div className="flex gap-4">
        <div className="flex w-14 shrink-0 flex-col items-center justify-center rounded-xl bg-stone-900 py-2 text-white">
          <span className="text-xs uppercase">{d.weekday}</span>
          <span className="text-2xl font-bold leading-none">{d.day}</span>
          <span className="text-xs uppercase">{d.month}</span>
        </div>
        <div className="min-w-0 flex-1 space-y-1">
          <h2 className="text-lg font-bold">{firingTitle(f)}</h2>
          <p className="text-sm text-stone-600">
            {d.time}
            {f.kiln_name && ` · ${f.kiln_name}`} · {people}
          </p>
          <span
            className={`inline-block rounded-full px-2.5 py-0.5 text-sm font-semibold ${
              inKiln ? 'bg-red-600 text-white' : PUBLIC_STYLES[f.status] ?? 'bg-stone-200 text-stone-700'
            }`}
          >
            {PUBLIC_LABELS[f.status] ?? STATUS_LABELS[f.status]}
          </span>
        </div>
      </div>

      {inKiln ? (
        <p className="text-stone-700">
          In the kiln now
          {f.estimated_unload_at && ` · est. unload ${formatWhen(f.estimated_unload_at, tz)}`}
        </p>
      ) : (
        <>
          <FullnessBar
            fullness={{ display_pct: f.fullness_pct, room_left_pct: f.room_left_pct, calculated_pct: f.fullness_pct, overridden: false }}
          />
          <p className="text-sm text-stone-600">
            {f.requests_open ? `Requests close ${formatWhen(f.requests_close_at, tz)}` : 'Requests closed'}
          </p>
          {f.requests_open ? (
            <Link to={`/firings/${f.id}?request=1`} className="btn bg-stone-900 text-white">
              Request space
            </Link>
          ) : (
            <Link to={`/firings/${f.id}`} className="btn border-2 border-stone-300 bg-white text-base">
              Details
            </Link>
          )}
        </>
      )}
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
    <section className="card space-y-2">
      <h2 className="font-bold">Kiln calendar</h2>
      <p className="text-sm text-stone-600">Subscribe to see upcoming firings in your calendar app.</p>
      <div className="flex flex-wrap gap-2">
        <a href={webcal} className="rounded-lg bg-stone-100 px-3 py-2 font-semibold text-orange-700">
          Add to calendar
        </a>
        <button onClick={() => void copy()} className="rounded-lg bg-stone-100 px-3 py-2 font-semibold text-stone-700">
          {copied ? 'Copied!' : 'Copy link'}
        </button>
      </div>
      <p className="break-all text-xs text-stone-500">{url}</p>
    </section>
  )
}
