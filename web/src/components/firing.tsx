import { useEffect, useRef, type CSSProperties, type ReactNode } from 'react'
import { Link } from 'react-router'
import { dayParts } from '../lib/time'
import type { Firing, FiringStatus, FiringSummary, Fullness, Kiln, Options, Participant } from '../lib/types'

/** The kiln's name, but only when there's more than one kiln (otherwise it's noise). */
export function kilnLabel(options: Options, kilnId: number): string | null {
  if (options.kilns.length < 2) return null
  return options.kilns.find((k) => k.id === kilnId)?.name ?? null
}

// ── Labels ──────────────────────────────────────────────────────────────────

export const STATUS_LABELS: Record<FiringStatus, string> = {
  proposed: 'Proposed',
  on_hold: 'On hold',
  confirmed: 'Confirmed',
  loading: 'Loading',
  loaded: 'Loaded',
  firing: 'Firing',
  cooling: 'Cooling',
  completed: 'Completed',
  cancelled: 'Cancelled',
}

const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

export function firingTitle(f: Pick<Firing, 'title' | 'cone' | 'firing_type'>): string {
  return f.title || `Cone ${f.cone} ${cap(f.firing_type)}`
}

/** Status lamp. Pass `label` for public wording ("Waiting for more work"). */
export function StatusBadge({ status, label }: { status: FiringStatus; label?: string }) {
  return (
    <span className="kc-status" data-status={status}>
      {label ?? STATUS_LABELS[status]}
    </span>
  )
}

// ── Fullness ────────────────────────────────────────────────────────────────

export function FullnessBar({
  fullness,
  large,
  caption = 'full',
}: {
  fullness: Fullness
  large?: boolean
  /** What the line under the meter says; `none` leaves it to the caller. */
  caption?: 'full' | 'room' | 'none'
}) {
  const pct = fullness.display_pct
  return (
    <div>
      <span
        className={`kc-meter${large ? ' kc-meter--lg' : ''}${pct >= 100 ? ' kc-meter--over' : ''}`}
        style={{ '--fill': pct } as CSSProperties}
        role="meter"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={pct}
        aria-label="Kiln fullness"
      >
        <span className="kc-meter__track">
          <span className="kc-meter__bar" />
        </span>
      </span>
      {caption !== 'none' && (
        <div className="kc-meter-caption">
          {caption === 'full' ? (
            <span>
              {pct}% full{fullness.overridden && <span className="kc-muted"> (set by kiln master)</span>}
            </span>
          ) : (
            <span>{fullness.room_left_pct}% room left</span>
          )}
        </div>
      )}
    </div>
  )
}

/** Who's in, under a firing's meter: each name with the share it takes up. */
export function Participants({ people }: { people: Participant[] }) {
  if (people.length === 0) return null
  return (
    <ul className="kc-people">
      {people.map((p) => (
        <li key={p.id}>
          {p.name} <span className="kc-people__share">{p.share_pct}%</span>
        </li>
      ))}
    </ul>
  )
}

/** "Our kiln is X" wide × Y" deep, about N shelves. One full shelf ≈ Z%." (§7) */
export function KilnFootnote({ kiln }: { kiln: Kiln }) {
  const parts: string[] = []
  if (kiln.interior_width_in && kiln.interior_depth_in) {
    parts.push(`Our kiln is ${kiln.interior_width_in}" wide × ${kiln.interior_depth_in}" deep`)
  }
  if (kiln.shelf_count) parts.push(`about ${kiln.shelf_count} shelves`)
  let text = parts.join(', ')
  if (text) text += '.'
  if (kiln.share_per_shelf_pct) text += ` One full shelf ≈ ${kiln.share_per_shelf_pct}%.`
  if (!text) text = 'Estimate how much of the kiln your work takes up. A kiln master will check.'
  return <p className="kc-caption kc-caption--body mt-1">{text.trim()}</p>
}

// ── Firing card (lists) ─────────────────────────────────────────────────────

export function FiringCard({ firing, options }: { firing: FiringSummary; options: Options }) {
  const d = dayParts(firing.scheduled_date)
  const kiln = kilnLabel(options, firing.kiln_id)
  return (
    <Link to={`/firings/${firing.id}`} className={`kc-plate kc-plate--link${firing.is_mine ? ' kc-plate--mine' : ''}`}>
      <div className="kc-plate__head">
        <span className="kc-label kc-label--date">
          {d.weekday} {d.month} {d.day}
        </span>
        <StatusBadge status={firing.status} />
      </div>
      <div className="kc-plate__body kc-plate__body--tight">
        <h3 className="kc-title kc-title--sm">
          {firingTitle(firing)}
          {firing.pending_count > 0 && (
            <span className="kc-tag kc-tag--flag ml-2 align-middle">{firing.pending_count} pending</span>
          )}
        </h3>
        {kiln && <p className="kc-caption kc-caption--body">{kiln}</p>}
        {firing.status !== 'cancelled' && (
          <>
            <FullnessBar fullness={firing.fullness} caption="none" />
            <Participants people={firing.participants} />
          </>
        )}
      </div>
    </Link>
  )
}

// ── Form bits ───────────────────────────────────────────────────────────────

export const inputClass = 'kc-input'
export const smallInputClass = 'kc-input kc-input--sm'

export function Field({ label, hint, children }: { label: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <label className="kc-field">
      <span className="kc-field__label">{label}</span>
      {children}
      {hint && <span className="kc-field__hint">{hint}</span>}
    </label>
  )
}

/** Big tappable share picker: quick presets, glove-friendly. */
export function ShareInput({ value, onChange, kiln }: { value: number; onChange: (v: number) => void; kiln: Kiln | undefined }) {
  const presets = [10, 25, 50, 75, 100]
  return (
    <div className="space-y-2">
      <div className="flex items-baseline justify-between gap-3">
        <span className="kc-field__label kc-field__label--flush">How much of the kiln?</span>
        <span className="kc-value">{value}%</span>
      </div>
      <div className="grid grid-cols-5 gap-2">
        {presets.map((p) => (
          <button
            key={p}
            type="button"
            onClick={() => onChange(p)}
            aria-pressed={value === p}
            className="kc-key kc-key--sm"
          >
            {p}%
          </button>
        ))}
      </div>
      {kiln && <KilnFootnote kiln={kiln} />}
    </div>
  )
}

export function ConeSelect({ value, onChange, cones, label = 'Cone' }: { value: string; onChange: (v: string) => void; cones: string[]; label?: string }) {
  return (
    <Field label={label}>
      <span className="kc-select-wrap">
        <select value={value} onChange={(e) => onChange(e.target.value)} className="kc-select">
          {cones.map((c) => (
            <option key={c} value={c}>
              Cone {c}
            </option>
          ))}
        </select>
      </span>
    </Field>
  )
}

export function Alert({ kind, children }: { kind: 'error' | 'warning' | 'info'; children: ReactNode }) {
  const styles = {
    error: 'kc-placard--refused',
    warning: 'kc-placard--caution',
    info: '',
  }[kind]
  const tag = { error: '!', warning: '!', info: 'Note' }[kind]
  return (
    <div role={kind === 'error' ? 'alert' : 'status'} className={`kc-placard ${styles}`}>
      <span className="kc-placard__tag" aria-hidden>
        {tag}
      </span>
      <div className="kc-placard__text">{children}</div>
    </div>
  )
}

/** Bottom sheet for short confirm/reason forms. */
export function Sheet({ title, open, onClose, children }: { title: string; open: boolean; onClose: () => void; children: ReactNode }) {
  const ref = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    const d = ref.current
    if (!d) return
    if (open && !d.open) d.showModal()
    if (!open && d.open) d.close()
  }, [open])
  return (
    <dialog
      ref={ref}
      onClose={onClose}
      onClick={(e) => e.target === ref.current && onClose()}
      className="kc-sheet"
    >
      {open && (
        <div className="kc-sheet__body">
          <div className="flex items-center justify-between gap-3">
            <h2 className="kc-title kc-title--sm">{title}</h2>
            <button type="button" onClick={onClose} className="kc-sheet__close" aria-label="Close">
              ✕
            </button>
          </div>
          {children}
        </div>
      )}
    </dialog>
  )
}
