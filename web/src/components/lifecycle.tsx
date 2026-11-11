import { useState, type FormEvent } from 'react'
import { firingActions } from '../lib/api'
import { formatWhen, fromInputValue, toInputValue } from '../lib/time'
import type { FiringDetail, FiringStatus, MutationResult, Options, TimestampField } from '../lib/types'
import { Alert, Field, inputClass, STATUS_LABELS } from './firing'

/** Button text per target status (§9B). */
export const STEP_LABELS: Partial<Record<FiringStatus, string>> = {
  loading: 'Start loading',
  loaded: 'Loaded',
  firing: 'START FIRING',
  cooling: 'Kiln off: cooling',
  unloaded: 'Unloaded',
  ready_for_pickup: 'Ready for pickup',
  completed: 'Completed',
}

/** Offers the next valid steps for this user and runs the confirm screen. */
export function LifecyclePanel({
  detail,
  options,
  source,
  onDone,
}: {
  detail: FiringDetail
  options: Options
  source: 'web' | 'kiln_qr'
  onDone: (res: MutationResult, step: FiringStatus) => void
}) {
  const [step, setStep] = useState<FiringStatus | null>(null)
  const { next_steps: steps, blocked_reason: blocked } = detail.can

  if (step) {
    return (
      <ConfirmStep
        detail={detail}
        options={options}
        step={step}
        source={source}
        onCancel={() => setStep(null)}
        onDone={(res) => {
          setStep(null)
          onDone(res, step)
        }}
      />
    )
  }
  if (steps.length === 0) {
    return blocked ? <Alert kind="info">{blocked}</Alert> : null
  }
  return (
    <div className="grid gap-3">
      {steps.map((s) => (
        <button
          key={s}
          onClick={() => setStep(s)}
          className={`btn ${s === 'firing' ? 'min-h-20 bg-red-600 text-2xl tracking-wide text-white' : 'bg-stone-900 text-white'}`}
        >
          {STEP_LABELS[s] ?? STATUS_LABELS[s]}
        </button>
      ))}
    </div>
  )
}

function nowInput(tz: string) {
  return toInputValue(new Date().toISOString(), tz)
}

function ConfirmStep({
  detail,
  options,
  step,
  source,
  onCancel,
  onDone,
}: {
  detail: FiringDetail
  options: Options
  step: FiringStatus
  source: 'web' | 'kiln_qr'
  onCancel: () => void
  onDone: (res: MutationResult) => void
}) {
  const tz = options.timezone
  const f = detail.firing
  const [when, setWhen] = useState(() => nowInput(tz))
  const [program, setProgram] = useState(options.programs[0]?.name ?? '')
  const [customProgram, setCustomProgram] = useState('')
  const [hours, setHours] = useState(() => {
    const d = options.programs[0]?.default_duration_minutes
    return d ? String(d / 60) : ''
  })
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  const isStart = step === 'firing'
  const programName = program === '__other' || options.programs.length === 0 ? customProgram.trim() : program
  const minutes = Math.round(Number(hours) * 60)
  const startISO = fromInputValue(when, tz)
  const estUnload =
    isStart && startISO && minutes > 0 ? new Date(new Date(startISO).getTime() + minutes * 60_000).toISOString() : null

  function pickProgram(name: string) {
    setProgram(name)
    const d = options.programs.find((p) => p.name === name)?.default_duration_minutes
    if (d) setHours(String(d / 60))
  }

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setErr(null)
    try {
      const res = await firingActions.advance(f.id, {
        to: step,
        // Only send a time when it was changed, so phone clock drift doesn't matter.
        occurred_at: detail.can.backdate && when !== nowInput(tz) ? startISO : null,
        program: isStart ? programName : null,
        estimated_duration_minutes: isStart ? minutes : null,
        note: note.trim() || null,
        source,
      })
      onDone(res)
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="card space-y-4 border-2 border-stone-900">
      <h2 className="text-xl font-bold">
        {isStart ? 'Start the firing' : `Mark as ${STATUS_LABELS[step].toLowerCase()}`}
      </h2>

      {isStart && (
        <>
          <Field label="Program">
            {options.programs.length > 0 && (
              <select value={program} onChange={(e) => pickProgram(e.target.value)} className={inputClass}>
                {options.programs.map((p) => (
                  <option key={p.name} value={p.name}>
                    {p.name}
                  </option>
                ))}
                <option value="__other">Other…</option>
              </select>
            )}
            {(program === '__other' || options.programs.length === 0) && (
              <input
                required
                maxLength={100}
                value={customProgram}
                onChange={(e) => setCustomProgram(e.target.value)}
                placeholder="e.g. Slow glaze cone 6"
                className={inputClass}
              />
            )}
          </Field>
          <Field label="Estimated duration (hours)">
            <input
              required
              type="number"
              inputMode="decimal"
              min={0.25}
              max={168}
              step={0.25}
              value={hours}
              onChange={(e) => setHours(e.target.value)}
              className={inputClass}
            />
          </Field>
        </>
      )}

      {detail.can.backdate && (
        <Field label="When" hint={<p className="mt-1 text-sm text-stone-500">Change this if it actually happened earlier.</p>}>
          <input type="datetime-local" required value={when} onChange={(e) => setWhen(e.target.value)} className={inputClass} />
        </Field>
      )}

      {estUnload && (
        <Alert kind="info">
          Estimated unload: <strong>{formatWhen(estUnload, tz)}</strong>
        </Alert>
      )}
      {step === 'ready_for_pickup' && <Alert kind="warning">Everyone with work in this firing will be told it's ready.</Alert>}

      {['loaded', 'cooling', 'unloaded', 'firing'].includes(step) && (
        <Field label="Note (optional)">
          <input value={note} maxLength={500} onChange={(e) => setNote(e.target.value)} className={inputClass} />
        </Field>
      )}

      {err && <Alert kind="error">{err}</Alert>}

      <button type="submit" disabled={busy} className={`btn text-white ${isStart ? 'bg-red-600' : 'bg-orange-600'}`}>
        {busy ? 'Saving…' : `Confirm: ${STEP_LABELS[step] ?? STATUS_LABELS[step]}`}
      </button>
      <button type="button" onClick={onCancel} className="btn text-base text-stone-600">
        Back
      </button>
    </form>
  )
}

// ── Recorded times + corrections ────────────────────────────────────────────

const TIME_ROWS: { field: TimestampField; label: string }[] = [
  { field: 'started_at', label: 'Started' },
  { field: 'cooling_at', label: 'Cooling' },
  { field: 'unloaded_at', label: 'Unloaded' },
  { field: 'ready_for_pickup_at', label: 'Ready for pickup' },
  { field: 'completed_at', label: 'Completed' },
]

/** Lists recorded lifecycle times; kiln masters can correct each one. */
export function RecordedTimes({
  detail,
  tz,
  onCorrect,
}: {
  detail: FiringDetail
  tz: string
  onCorrect: (field: TimestampField) => void
}) {
  const f = detail.firing
  const rows = TIME_ROWS.filter((r) => f[r.field])
  if (rows.length === 0) return null
  return (
    <div className="card divide-y divide-stone-200 py-2">
      {f.program && (
        <p className="py-2 text-stone-700">
          Program: <strong>{f.program}</strong>
          {f.estimated_duration_minutes && ` · about ${Math.round((f.estimated_duration_minutes / 60) * 10) / 10}h`}
        </p>
      )}
      {f.estimated_unload_at && !f.unloaded_at && (
        <p className="py-2">
          Estimated unload: <strong>{formatWhen(f.estimated_unload_at, tz)}</strong>
        </p>
      )}
      {rows.map((r) => (
        <div key={r.field} className="flex items-center justify-between gap-2 py-2">
          <span>
            <span className="text-stone-500">{r.label}:</span> {formatWhen(f[r.field]!, tz)}
          </span>
          {detail.can.correct_timestamps && (
            <button onClick={() => onCorrect(r.field)} className="rounded-lg px-3 py-2 text-sm font-semibold text-orange-700">
              Fix
            </button>
          )}
        </div>
      ))}
    </div>
  )
}

export function CorrectTimeForm({
  detail,
  field,
  tz,
  busy,
  onSubmit,
}: {
  detail: FiringDetail
  field: TimestampField
  tz: string
  busy: boolean
  onSubmit: (iso: string) => void
}) {
  const current = detail.firing[field]
  const [value, setValue] = useState(current ? toInputValue(current, tz) : '')
  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        const iso = fromInputValue(value, tz)
        if (iso) onSubmit(iso)
      }}
    >
      <Field label={TIME_ROWS.find((r) => r.field === field)?.label ?? field}>
        <input type="datetime-local" required value={value} onChange={(e) => setValue(e.target.value)} className={inputClass} />
      </Field>
      {field === 'started_at' && detail.firing.estimated_duration_minutes && (
        <p className="text-sm text-stone-600">The estimated unload time moves with it.</p>
      )}
      <button type="submit" disabled={busy} className="btn bg-orange-600 text-white">
        {busy ? 'Saving…' : 'Save time'}
      </button>
    </form>
  )
}
