import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import { Alert, ConeSelect, Field, inputClass, ShareInput } from '../components/firing'
import { createFiring, getOptions } from '../lib/api'
import { PageSpinner, useSession } from '../lib/session'
import { earliestDay } from '../lib/time'
import type { Options } from '../lib/types'
import { useLoad } from '../lib/useLoad'

export default function NewFiringPage() {
  const { data: options, error } = useLoad(getOptions, [])
  if (error) return <Alert kind="error">{error}</Alert>
  if (!options) return <PageSpinner />
  return <ProposeForm options={options} />
}


function ProposeForm({ options }: { options: Options }) {
  const { me } = useSession()
  const navigate = useNavigate()
  const canBypass = !!me && (me.roles.kiln_master || me.roles.admin)

  const activeKilns = options.kilns.filter((k) => k.active)
  const [kilnId, setKilnId] = useState(activeKilns[0]?.id ?? 0)
  const [firingType, setFiringType] = useState(options.firing_types.includes('glaze') ? 'glaze' : options.firing_types[0]!)
  const [cone, setCone] = useState(options.cones.includes('6') ? '6' : options.cones[0]!)
  // Firings are booked by day. Members start from the earliest day their notice allows.
  const earliest = earliestDay(options.timezone, options.min_notice_hours)
  const [day, setDay] = useState(earliest)
  const [description, setDescription] = useState('')
  const [share, setShare] = useState(25)
  const [clayBody, setClayBody] = useState('')
  const [title, setTitle] = useState('')
  const [notes, setNotes] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  // Days this kiln is blacked out (maintenance and other needs). YYYY-MM-DD strings compare correctly.
  const blackouts = options.blackouts.filter((b) => b.kiln_id === kilnId)
  const blocked = blackouts.some((b) => day >= b.start_date && day <= b.end_date)

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!day) return setErr('Pick a day.')
    setBusy(true)
    setErr(null)
    try {
      const d = await createFiring({
        kiln_id: kilnId,
        firing_type: firingType,
        cone,
        title: title || null,
        notes,
        scheduled_date: day,
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
      <h1 className="kc-title">Propose a firing</h1>

      {activeKilns.length === 0 && <Alert kind="error">There are no active kilns. Ask an admin.</Alert>}
      {activeKilns.length > 1 && (
        <fieldset>
          <legend className="kc-field__label">Kiln</legend>
          <div className="kc-keys kc-keys--fill">
            {activeKilns.map((k) => (
              <button
                key={k.id}
                type="button"
                aria-pressed={kilnId === k.id}
                onClick={() => setKilnId(k.id)}
                className="kc-key"
              >
                {k.name}
              </button>
            ))}
          </div>
        </fieldset>
      )}

      <fieldset>
        <legend className="kc-field__label">Type</legend>
        <div className="kc-keys kc-keys--fill">
          {options.firing_types.map((t) => (
            <button
              key={t}
              type="button"
              aria-pressed={firingType === t}
              onClick={() => setFiringType(t)}
              className="kc-key capitalize"
            >
              {t}
            </button>
          ))}
        </div>
      </fieldset>

      <ConeSelect value={cone} onChange={setCone} cones={options.cones} />

      <Field
        label="Day"
        hint={
          <span className="kc-caption kc-caption--body block">
            Booked for a day; the kiln runs sometime that day. Requests close {options.request_cutoff_hours}h before.
            {blackouts.length > 0 && (
              <>
                {' '}
                Unavailable:{' '}
                {blackouts
                  .map((b) => (b.start_date === b.end_date ? b.start_date : `${b.start_date}–${b.end_date}`))
                  .join(', ')}
                .
              </>
            )}
          </span>
        }
      >
        <input
          type="date"
          required
          min={canBypass ? undefined : earliest}
          value={day}
          onChange={(e) => setDay(e.target.value)}
          className={inputClass}
        />
      </Field>

      {blocked && <Alert kind="warning">That day is unavailable for this kiln. Pick another day.</Alert>}

      <section className="space-y-4">
        <h2 className="kc-title kc-title--sm">Your work</h2>
        <Field label="What are you putting in?">
          <textarea
            required
            rows={2}
            maxLength={1000}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="4 mugs, 2 medium bowls, 1 tall vase"
            className="kc-textarea"
          />
        </Field>
        <ShareInput value={share} onChange={setShare} kiln={activeKilns.find((k) => k.id === kilnId)} />
        <Field label="Clay body (optional)">
          <input value={clayBody} maxLength={120} onChange={(e) => setClayBody(e.target.value)} className={inputClass} />
        </Field>
      </section>

      <details className="kc-plate kc-plate--pad">
        <summary className="kc-field__label cursor-pointer">More (title, notes)</summary>
        <div className="mt-4 space-y-4">
          <Field label="Title (optional)">
            <input value={title} maxLength={120} onChange={(e) => setTitle(e.target.value)} placeholder={`Cone ${cone} ${firingType}`} className={inputClass} />
          </Field>
          <Field label="Notes (optional)">
            <textarea rows={3} maxLength={2000} value={notes} onChange={(e) => setNotes(e.target.value)} className="kc-textarea" />
          </Field>
        </div>
      </details>

      {err && <Alert kind="error">{err}</Alert>}

      <button type="submit" disabled={busy || blocked || activeKilns.length === 0} className="kc-key kc-key--block kc-key--fire">
        {busy ? 'Proposing…' : 'Propose firing'}
      </button>
    </form>
  )
}
