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
  completed: 'Unloaded: ready for pickup',
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
    <div className="kc-keys">
      {steps.map((s) => (
        <button
          key={s}
          onClick={() => setStep(s)}
          className={`kc-key kc-key--block ${s === 'firing' ? 'kc-key--fire kc-key--lg' : ''}`}
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
    <form onSubmit={submit} className="kc-plate kc-plate--pad kc-plate--strong space-y-4">
      <h2 className="kc-title kc-title--sm">
        {isStart ? 'Start the firing' : step === 'completed' ? 'Unloaded and ready for pickup' : `Mark as ${STATUS_LABELS[step].toLowerCase()}`}
      </h2>

      {isStart && (
        <>
          <Field label="Program">
            {options.programs.length > 0 && (
              <span className="kc-select-wrap">
                <select value={program} onChange={(e) => pickProgram(e.target.value)} className="kc-select">
                  {options.programs.map((p) => (
                    <option key={p.name} value={p.name}>
                      {p.name}
                    </option>
                  ))}
                  <option value="__other">Other…</option>
                </select>
              </span>
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
              className={`${inputClass} kc-input--number`}
            />
          </Field>
        </>
      )}

      {detail.can.backdate && (
        <Field label="When" hint={<span className="kc-caption kc-caption--body block">Change this if it actually happened earlier.</span>}>
          <input type="datetime-local" required value={when} onChange={(e) => setWhen(e.target.value)} className={inputClass} />
        </Field>
      )}

      {estUnload && (
        <Alert kind="info">
          Estimated unload: <strong>{formatWhen(estUnload, tz)}</strong>
        </Alert>
      )}
      {step === 'completed' && <Alert kind="warning">Everyone with work in this firing will be told it's ready for pickup.</Alert>}

      {['loaded', 'cooling', 'completed', 'firing'].includes(step) && (
        <Field label="Note (optional)">
          <input value={note} maxLength={500} onChange={(e) => setNote(e.target.value)} className={inputClass} />
        </Field>
      )}

      {err && <Alert kind="error">{err}</Alert>}

      <button type="submit" disabled={busy} className={`kc-key kc-key--block ${isStart ? 'kc-key--fire' : ''}`}>
        {busy ? 'Saving…' : `Confirm: ${STEP_LABELS[step] ?? STATUS_LABELS[step]}`}
      </button>
      <button type="button" onClick={onCancel} className="kc-key kc-key--block">
        Back
      </button>
    </form>
  )
}

// ── Recorded times + corrections ────────────────────────────────────────────

export const TIME_ROWS: { field: TimestampField; label: string }[] = [
  { field: 'started_at', label: 'Started' },
  { field: 'cooling_at', label: 'Cooling' },
  { field: 'unloaded_at', label: 'Unloaded' },
]

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
        <p className="kc-caption kc-caption--body">The estimated unload time moves with it.</p>
      )}
      <button type="submit" disabled={busy} className="kc-key kc-key--block kc-key--fire">
        {busy ? 'Saving…' : 'Save time'}
      </button>
    </form>
  )
}
