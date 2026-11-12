import { useState } from 'react'
import { Link, Navigate, useParams, useSearchParams } from 'react-router'
import { Alert, firingTitle, kilnLabel, StatusBadge, STATUS_LABELS } from '../components/firing'
import { LifecyclePanel, STEP_LABELS } from '../components/lifecycle'
import { getFiring, getKilnView, getOptions } from '../lib/api'
import { PageSpinner } from '../lib/session'
import { formatDate, formatWhen } from '../lib/time'
import type { FiringStatus, MutationResult } from '../lib/types'
import { useLoad } from '../lib/useLoad'

/**
 * /kiln: where the kiln QR code points (§9B). Each kiln's code can carry its id
 * (/kiln?k=2). One relevant firing → straight to it; several → pick list.
 */
export function KilnPage() {
  const [params] = useSearchParams()
  const k = params.get('k')
  const { data, error } = useLoad(() => Promise.all([getKilnView(k), getOptions()]), [k])
  if (error) return <Alert kind="error">{error === 'not found' ? 'Unknown kiln. Is the QR code right?' : error}</Alert>
  if (!data) return <PageSpinner />
  const [view, options] = data
  const suffix = k ? `?k=${encodeURIComponent(k)}` : ''

  if (view.firings.length === 1) return <Navigate to={`/kiln/${view.firings[0]!.id}${suffix}`} replace />

  return (
    <div className="space-y-5">
      <h1 className="text-2xl font-bold">{view.kiln ? view.kiln.name : 'Kiln check-in'}</h1>
      {view.firings.length === 0 ? (
        <div className="card space-y-4 text-center">
          <p className="text-xl font-semibold">No active firing</p>
          <p className="text-stone-600">Nothing is loading or running, and nothing confirmed starts within a day.</p>
          <Link to="/" className="btn bg-orange-600 text-white">
            See the schedule
          </Link>
        </div>
      ) : (
        <>
          <p className="text-stone-600">Which firing?</p>
          <div className="space-y-3">
            {view.firings.map((f) => (
              <Link key={f.id} to={`/kiln/${f.id}${suffix}`} className="card block space-y-1 active:bg-stone-50">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-lg font-bold">{firingTitle(f)}</span>
                  <StatusBadge status={f.status} />
                </div>
                <p className="text-stone-600">
                  {formatDate(f.scheduled_date)}
                  {!view.kiln && kilnLabel(options, f.kiln_id) && ` · ${kilnLabel(options, f.kiln_id)}`} · {f.proposer_name}
                </p>
              </Link>
            ))}
          </div>
        </>
      )}
    </div>
  )
}

/** /kiln/:id: big summary + only the next valid actions for this user. */
export function KilnFiringPage() {
  const id = Number(useParams().id)
  const [params] = useSearchParams()
  const k = params.get('k')
  const { data, error, setData } = useLoad(() => Promise.all([getFiring(id), getOptions()]), [id])
  const [done, setDone] = useState<FiringStatus | null>(null)

  if (error) return <Alert kind="error">{error === 'not found' ? 'That firing does not exist.' : error}</Alert>
  if (!data) return <PageSpinner />
  const [detail, options] = data
  const f = detail.firing
  const tz = options.timezone

  function onDone(res: MutationResult, step: FiringStatus) {
    setData([res, options])
    setDone(step)
    window.scrollTo({ top: 0 })
  }

  return (
    <div className="space-y-5">
      {done && (
        <div role="status" className="rounded-2xl bg-green-700 p-5 text-center text-white">
          <p className="text-3xl">✓</p>
          <p className="text-xl font-bold">{STEP_LABELS[done] ?? STATUS_LABELS[done]}</p>
          <p className="mt-1 opacity-90">Saved {formatWhen(new Date().toISOString(), tz)}</p>
        </div>
      )}

      <section className="card space-y-2">
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-2xl font-bold">{firingTitle(f)}</h1>
          <StatusBadge status={f.status} />
        </div>
        <p className="text-lg">{formatDate(f.scheduled_date)}</p>
        <p className="text-stone-600">
          {kilnLabel(options, f.kiln_id) && `${kilnLabel(options, f.kiln_id)} · `}
          {f.fullness.display_pct}% full · {1 + detail.requests.filter((r) => r.status === 'approved').length} people
        </p>
        {f.status === 'firing' && f.estimated_unload_at && (
          <p className="text-lg">
            Estimated unload: <strong>{formatWhen(f.estimated_unload_at, tz)}</strong>
          </p>
        )}
        {f.program && <p className="text-stone-600">Program: {f.program}</p>}
      </section>

      {!done && <LifecyclePanel detail={detail} options={options} source="kiln_qr" onDone={onDone} />}
      {done && detail.can.next_steps.length > 0 && (
        <button onClick={() => setDone(null)} className="btn border-2 border-stone-900 bg-white">
          Next step
        </button>
      )}

      <div className="grid grid-cols-2 gap-3">
        <Link to={`/firings/${f.id}`} className="btn border-2 border-stone-300 bg-white text-base">
          Firing details
        </Link>
        <Link to={`/kiln${k ? `?k=${encodeURIComponent(k)}` : ''}`} className="btn border-2 border-stone-300 bg-white text-base">
          Other firings
        </Link>
      </div>
    </div>
  )
}
