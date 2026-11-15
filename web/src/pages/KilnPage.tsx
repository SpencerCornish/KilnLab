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
      <h1 className="kc-title">{view.kiln ? view.kiln.name : 'Kiln check-in'}</h1>
      {view.firings.length === 0 ? (
        <div className="kc-plate kc-plate--pad space-y-4 text-center">
          <p className="kc-title kc-title--sm">No active firing</p>
          <p className="kc-caption kc-caption--body">Nothing is loading or running, and nothing confirmed starts within a day.</p>
          <Link to="/" className="kc-key kc-key--block kc-key--fire">
            See the schedule
          </Link>
        </div>
      ) : (
        <>
          <p className="kc-caption kc-caption--body">Which firing?</p>
          <div className="space-y-3">
            {view.firings.map((f) => (
              <Link key={f.id} to={`/kiln/${f.id}${suffix}`} className="kc-plate kc-plate--link">
                <div className="kc-plate__head">
                  <span className="kc-label kc-label--date">{formatDate(f.scheduled_date)}</span>
                  <StatusBadge status={f.status} />
                </div>
                <div className="kc-plate__body kc-plate__body--tight">
                  <p className="kc-title kc-title--sm">{firingTitle(f)}</p>
                  <p className="kc-caption kc-caption--body">
                    {!view.kiln && kilnLabel(options, f.kiln_id) && `${kilnLabel(options, f.kiln_id)} · `}
                    {f.proposer_name}
                  </p>
                </div>
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
        <div role="status" className="kc-placard kc-placard--ok">
          <span className="kc-placard__tag" aria-hidden>
            ✓
          </span>
          <div className="kc-placard__text">
            <p className="kc-title kc-title--sm">{STEP_LABELS[done] ?? STATUS_LABELS[done]}</p>
            <p className="kc-caption kc-caption--body">Saved {formatWhen(new Date().toISOString(), tz)}</p>
          </div>
        </div>
      )}

      <section className="kc-plate">
        <header className="kc-plate__head">
          <span className="kc-label kc-label--date">{formatDate(f.scheduled_date)}</span>
          <StatusBadge status={f.status} />
        </header>
        <div className="kc-plate__body kc-plate__body--tight">
          <h1 className="kc-title kc-title--sm">{firingTitle(f)}</h1>
          <p className="kc-data">
            {kilnLabel(options, f.kiln_id) && `${kilnLabel(options, f.kiln_id)} · `}
            {f.fullness.display_pct}% full · {1 + detail.requests.filter((r) => r.status === 'approved').length} people
          </p>
          {f.status === 'firing' && f.estimated_unload_at && (
            <div className="kc-readout">
              <span className="kc-readout__label">Estimated unload</span>
              <span className="kc-readout__value">{formatWhen(f.estimated_unload_at, tz)}</span>
            </div>
          )}
          {f.program && <p className="kc-data kc-muted">Program: {f.program}</p>}
        </div>
      </section>

      {!done && <LifecyclePanel detail={detail} options={options} source="kiln_qr" onDone={onDone} />}
      {done && detail.can.next_steps.length > 0 && (
        <button onClick={() => setDone(null)} className="kc-key kc-key--block">
          Next step
        </button>
      )}

      <div className="kc-keys kc-keys--fill">
        <Link to={`/firings/${f.id}`} className="kc-key">
          Firing details
        </Link>
        <Link to={`/kiln${k ? `?k=${encodeURIComponent(k)}` : ''}`} className="kc-key">
          Other firings
        </Link>
      </div>
    </div>
  )
}
