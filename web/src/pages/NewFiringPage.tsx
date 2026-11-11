import { addHours, setHours, startOfHour } from 'date-fns'
import { TZDate } from '@date-fns/tz'
import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import { Alert, ConeSelect, Field, inputClass, ShareInput } from '../components/firing'
import { createFiring, getOptions } from '../lib/api'
import { PageSpinner, useSession } from '../lib/session'
import { fromInputValue, toInputValue } from '../lib/time'
import type { Options } from '../lib/types'
import { useLoad } from '../lib/useLoad'

export default function NewFiringPage() {
  const { data: options, error } = useLoad(getOptions, [])
  if (error) return <Alert kind="error">{error}</Alert>
  if (!options) return <PageSpinner />
  return <ProposeForm options={options} />
}

/** First 9am in the studio timezone that satisfies the minimum notice. */
function defaultStart(o: Options): string {
  const earliest = startOfHour(addHours(TZDate.tz(o.timezone), o.min_notice_hours + 1))
  let d = setHours(earliest, 9)
  if (d < earliest) d = addHours(d, 24)
  return toInputValue(d.toISOString(), o.timezone)
}

function ProposeForm({ options }: { options: Options }) {
  const { me } = useSession()
  const navigate = useNavigate()
  const canBypass = !!me && (me.roles.kiln_master || me.roles.admin)

  const activeKilns = options.kilns.filter((k) => k.active)
  const [kilnId, setKilnId] = useState(activeKilns[0]?.id ?? 0)
  const [firingType, setFiringType] = useState(options.firing_types.includes('glaze') ? 'glaze' : options.firing_types[0]!)
  const [cone, setCone] = useState(options.cones.includes('6') ? '6' : options.cones[0]!)
  const [start, setStart] = useState(() => defaultStart(options))
  const [description, setDescription] = useState('')
  const [share, setShare] = useState(25)
  const [clayBody, setClayBody] = useState('')
  const [title, setTitle] = useState('')
  const [notes, setNotes] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  async function submit(e: FormEvent) {
    e.preventDefault()
    const iso = fromInputValue(start, options.timezone)
    if (!iso) return setErr('Pick a date and time.')
    setBusy(true)
    setErr(null)
    try {
      const d = await createFiring({
        kiln_id: kilnId,
        firing_type: firingType,
        cone,
        title: title || null,
        notes,
        scheduled_start: iso,
        description,
        share_pct: share,
        proposer_cone: cone,
        clay_body: clayBody || null,
      })
      navigate(`/firings/${d.firing.id}`, { replace: true })
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="space-y-5">
      <h1 className="text-2xl font-bold">Propose a firing</h1>

      {activeKilns.length === 0 && <Alert kind="error">There are no active kilns. Ask an admin.</Alert>}
      {activeKilns.length > 1 && (
        <fieldset>
          <legend className="font-semibold">Kiln</legend>
          <div className="mt-1 grid gap-2">
            {activeKilns.map((k) => (
              <button
                key={k.id}
                type="button"
                onClick={() => setKilnId(k.id)}
                className={`rounded-xl border-2 px-4 py-3 text-left text-lg font-semibold ${kilnId === k.id ? 'border-orange-600 bg-orange-50' : 'border-stone-300 bg-white'}`}
              >
                {k.name}
              </button>
            ))}
          </div>
        </fieldset>
      )}

      <fieldset>
        <legend className="font-semibold">Type</legend>
        <div className="mt-1 grid grid-cols-3 gap-2">
          {options.firing_types.map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => setFiringType(t)}
              className={`rounded-xl border-2 py-3 text-lg font-semibold capitalize ${firingType === t ? 'border-orange-600 bg-orange-50' : 'border-stone-300 bg-white'}`}
            >
              {t}
            </button>
          ))}
        </div>
      </fieldset>

      <ConeSelect value={cone} onChange={setCone} cones={options.cones} />

      <Field
        label="Start"
        hint={
          <p className="mt-1 text-sm text-stone-500">
            {options.timezone.replace('_', ' ')} time.{' '}
            {canBypass
              ? 'As a kiln master you can skip the minimum notice.'
              : `At least ${options.min_notice_hours} hours from now.`}{' '}
            Requests close {options.request_cutoff_hours}h before the start.
          </p>
        }
      >
        <input type="datetime-local" required value={start} onChange={(e) => setStart(e.target.value)} className={inputClass} />
      </Field>

      <div className="card space-y-4">
        <h2 className="text-lg font-bold">Your work</h2>
        <Field label="What are you putting in?">
          <textarea
            required
            rows={2}
            maxLength={1000}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="4 mugs, 2 medium bowls, 1 tall vase"
            className={inputClass}
          />
        </Field>
        <ShareInput value={share} onChange={setShare} kiln={activeKilns.find((k) => k.id === kilnId)} />
        <Field label="Clay body (optional)">
          <input value={clayBody} maxLength={120} onChange={(e) => setClayBody(e.target.value)} className={inputClass} />
        </Field>
      </div>

      <details className="card">
        <summary className="cursor-pointer font-semibold">More (title, notes)</summary>
        <div className="mt-4 space-y-4">
          <Field label="Title (optional)">
            <input value={title} maxLength={120} onChange={(e) => setTitle(e.target.value)} placeholder={`Cone ${cone} ${firingType}`} className={inputClass} />
          </Field>
          <Field label="Notes (optional)">
            <textarea rows={3} maxLength={2000} value={notes} onChange={(e) => setNotes(e.target.value)} className={inputClass} />
          </Field>
        </div>
      </details>

      {err && <Alert kind="error">{err}</Alert>}

      <button type="submit" disabled={busy || activeKilns.length === 0} className="btn bg-orange-600 text-white">
        {busy ? 'Proposing…' : 'Propose firing'}
      </button>
    </form>
  )
}
