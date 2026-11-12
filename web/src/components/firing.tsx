import { useEffect, useRef, type ReactNode } from 'react'
import { Link } from 'react-router'
import { dayParts } from '../lib/time'
import type { Firing, FiringStatus, FiringSummary, Fullness, Kiln, Options } from '../lib/types'

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

const STATUS_STYLES: Record<FiringStatus, string> = {
  proposed: 'bg-sky-100 text-sky-900',
  on_hold: 'bg-amber-100 text-amber-900',
  confirmed: 'bg-green-100 text-green-900',
  loading: 'bg-indigo-100 text-indigo-900',
  loaded: 'bg-indigo-100 text-indigo-900',
  firing: 'bg-red-600 text-white',
  cooling: 'bg-blue-100 text-blue-900',
  completed: 'bg-stone-200 text-stone-700',
  cancelled: 'bg-stone-200 text-stone-500 line-through',
}

const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

export function firingTitle(f: Pick<Firing, 'title' | 'cone' | 'firing_type'>): string {
  return f.title || `Cone ${f.cone} ${cap(f.firing_type)}`
}

export function StatusBadge({ status }: { status: FiringStatus }) {
  return (
    <span className={`inline-block rounded-full px-2.5 py-0.5 text-sm font-semibold ${STATUS_STYLES[status]}`}>
      {STATUS_LABELS[status]}
    </span>
  )
}

// ── Fullness ────────────────────────────────────────────────────────────────

export function FullnessBar({ fullness, large }: { fullness: Fullness; large?: boolean }) {
  const pct = fullness.display_pct
  const color = pct >= 100 ? 'bg-red-600' : pct >= 75 ? 'bg-orange-500' : 'bg-green-600'
  return (
    <div>
      <div
        className={`overflow-hidden rounded-full bg-stone-200 ${large ? 'h-5' : 'h-2.5'}`}
        role="meter"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={pct}
        aria-label="Kiln fullness"
      >
        <div className={`h-full ${color}`} style={{ width: `${pct}%` }} />
      </div>
      <div className={`mt-1 flex justify-between ${large ? 'text-base' : 'text-sm'} text-stone-600`}>
        <span>
          <strong className="text-stone-900">{pct}% full</strong>
          {fullness.overridden && <span className="ml-1 italic">(set by kiln master)</span>}
        </span>
        <span>{fullness.room_left_pct}% room left</span>
      </div>
    </div>
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
  return <p className="mt-1 text-sm italic text-stone-500">{text.trim()}</p>
}

// ── Firing card (lists) ─────────────────────────────────────────────────────

export function FiringCard({ firing, options }: { firing: FiringSummary; options: Options }) {
  const d = dayParts(firing.scheduled_date)
  const kiln = kilnLabel(options, firing.kiln_id)
  return (
    <Link to={`/firings/${firing.id}`} className="card flex gap-4 active:bg-stone-50">
      <div className="flex w-14 shrink-0 flex-col items-center justify-center rounded-xl bg-stone-900 py-2 text-white">
        <span className="text-xs uppercase">{d.weekday}</span>
        <span className="text-2xl font-bold leading-none">{d.day}</span>
        <span className="text-xs uppercase">{d.month}</span>
      </div>
      <div className="min-w-0 flex-1 space-y-2">
        <div className="flex flex-wrap items-center gap-2">
          <h3 className="truncate text-lg font-bold">{firingTitle(firing)}</h3>
          <StatusBadge status={firing.status} />
        </div>
        <p className="text-sm text-stone-600">
          {kiln && `${kiln} · `}by {firing.proposer.name}
          {firing.pending_count > 0 && (
            <span className="ml-2 font-semibold text-orange-700">{firing.pending_count} pending</span>
          )}
        </p>
        {firing.status !== 'cancelled' && <FullnessBar fullness={firing.fullness} />}
      </div>
    </Link>
  )
}

// ── Form bits ───────────────────────────────────────────────────────────────

export const inputClass =
  'mt-1 block w-full rounded-xl border-2 border-stone-300 bg-white px-3 py-3 text-lg focus:border-orange-600 focus:outline-none'

export function Field({ label, hint, children }: { label: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <label className="block">
      <span className="font-semibold">{label}</span>
      {children}
      {hint}
    </label>
  )
}

/** Big tappable share picker: a slider plus quick buttons. */
export function ShareInput({ value, onChange, kiln }: { value: number; onChange: (v: number) => void; kiln: Kiln | undefined }) {
  const presets = [10, 25, 50, 75, 100]
  return (
    <div>
      <div className="flex items-baseline justify-between">
        <span className="font-semibold">How much of the kiln?</span>
        <span className="text-2xl font-bold tabular-nums">{value}%</span>
      </div>
      <input
        type="range"
        min={1}
        max={100}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="mt-2 h-10 w-full accent-orange-600"
        aria-label="Share of the kiln in percent"
      />
      <div className="mt-1 grid grid-cols-5 gap-2">
        {presets.map((p) => (
          <button
            key={p}
            type="button"
            onClick={() => onChange(p)}
            className={`rounded-lg border-2 py-2 font-semibold ${value === p ? 'border-orange-600 bg-orange-50' : 'border-stone-300 bg-white'}`}
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
      <select value={value} onChange={(e) => onChange(e.target.value)} className={inputClass}>
        {cones.map((c) => (
          <option key={c} value={c}>
            Cone {c}
          </option>
        ))}
      </select>
    </Field>
  )
}

export function Alert({ kind, children }: { kind: 'error' | 'warning' | 'info'; children: ReactNode }) {
  const styles = {
    error: 'border-red-300 bg-red-50 text-red-900',
    warning: 'border-amber-300 bg-amber-50 text-amber-900',
    info: 'border-sky-300 bg-sky-50 text-sky-900',
  }[kind]
  return (
    <div role={kind === 'error' ? 'alert' : 'status'} className={`rounded-xl border-2 p-3 font-medium ${styles}`}>
      {children}
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
      className="mx-auto mb-0 mt-auto w-full max-w-xl rounded-t-2xl bg-white p-0 backdrop:bg-black/50"
    >
      {open && (
        <div className="space-y-4 p-5 pb-[max(1.25rem,env(safe-area-inset-bottom))]">
          <div className="flex items-center justify-between">
            <h2 className="text-xl font-bold">{title}</h2>
            <button type="button" onClick={onClose} className="rounded-lg px-3 py-2 text-stone-500" aria-label="Close">
              ✕
            </button>
          </div>
          {children}
        </div>
      )}
    </dialog>
  )
}
