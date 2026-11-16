import { useState, type FormEvent } from 'react'
import { firingActions } from '../lib/api'
import { formatWhen, fromInputValue, toInputValue } from '../lib/time'
import type { FiringDetail, FiringStatus, MutationResult, Options, TimestampField } from '../lib/types'
import { Alert, Field, inputClass, STATUS_LABELS } from './firing'

/** Button text per target status (§9B). */
export const STEP_LABELS: Partial<Record<FiringStatus, string>> = {
  firing: 'START FIRING',
  completed: 'Unloaded: ready for pickup',
}

/** Permanent sub-text under each step button; explains what the action records. */
const STEP_HINTS: Partial<Record<FiringStatus, string>> = {
  firing: 'Records the start; cool and unload times follow.',
  completed: 'Tells everyone their work is ready to collect.',
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
          className={`kc-key kc-key--block kc-key--sub ${s === 'firing' ? 'kc-key--fire kc-key--lg' : ''}`}
        >
          {STEP_LABELS[s] ?? STATUS_LABELS[s]}
          {STEP_HINTS[s] && <span className="kc-key__sub">{STEP_HINTS[s]}</span>}
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
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  const isStart = step === 'firing'
  const startISO = fromInputValue(when, tz)
  const estimatedStart = isStart ? startISO : f.started_at
  const firingEnds =
    isStart && startISO ? new Date(new Date(startISO).getTime() + f.firing_minutes * 60_000).toISOString() : f.firing_ends_at
  const unloadable =
    isStart && startISO
      ? new Date(new Date(startISO).getTime() + (f.firing_minutes + f.cooldown_minutes) * 60_000).toISOString()
      : f.unloadable_at

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setErr(null)
    try {
      const res = await firingActions.advance(f.id, {
        to: step,
        // Only send a time when it was changed, so phone clock drift doesn't matter.
        occurred_at: detail.can.backdate && when !== nowInput(tz) ? startISO : null,
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
        {isStart ? 'Start the firing' : 'Unloaded and ready for pickup'}
      </h2>

      <p className="kc-caption kc-caption--body">
        {f.title} · about {Math.round((f.firing_minutes / 60) * 10) / 10}h firing +{' '}
        {Math.round((f.cooldown_minutes / 60) * 10) / 10}h cooldown
      </p>

      {isStart && firingEnds && unloadable && (
        <Alert kind="info">
          Kiln off around <strong>{formatWhen(firingEnds, tz)}</strong>; ready to unload around{' '}
          <strong>{formatWhen(unloadable, tz)}</strong>.
        </Alert>
      )}
      {estimatedStart && !isStart && (
        <p className="kc-caption kc-caption--body">
          Started {formatWhen(estimatedStart, tz)}
          {unloadable && ` · estimated ready ${formatWhen(unloadable, tz)}`}
        </p>
      )}

      {detail.can.backdate && (
        <Field label="When" hint={<span className="kc-caption kc-caption--body block">Change this if it actually happened earlier.</span>}>
          <input type="datetime-local" required value={when} onChange={(e) => setWhen(e.target.value)} className={inputClass} />
        </Field>
      )}

      {step === 'completed' && (
        <Alert kind="warning">
          Everyone with work in this firing will be told it's ready for pickup.
          {unloadable && !f.ready_to_unload && ' The estimate says it may still be hot; unload only if the kiln is cool.'}
        </Alert>
      )}

      {['completed', 'firing'].includes(step) && (
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
      {field === 'started_at' && (
        <p className="kc-caption kc-caption--body">The estimated cool and unload times move with it.</p>
      )}
      <button type="submit" disabled={busy} className="kc-key kc-key--block kc-key--fire">
        {busy ? 'Saving…' : 'Save time'}
      </button>
    </form>
  )
}
