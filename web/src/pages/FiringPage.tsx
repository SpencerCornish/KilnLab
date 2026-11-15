import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { Link, useParams, useSearchParams } from 'react-router'
import {
  Alert,
  ConeSelect,
  Field,
  firingTitle,
  FullnessBar,
  inputClass,
  kilnLabel,
  ShareInput,
  Sheet,
  STATUS_LABELS,
  StatusBadge,
} from '../components/firing'
import { firingActions as act, getFiring, getOptions } from '../lib/api'
import { PageSpinner } from '../lib/session'
import { formatDate, formatWhen, relative } from '../lib/time'
import type {
  FiringDetail,
  FiringEvent,
  FiringRequest,
  MutationResult,
  Options,
  RequestInput,
  TimestampField,
} from '../lib/types'
import { useLoad } from '../lib/useLoad'
import { CorrectTimeForm, LifecyclePanel, TIME_ROWS } from '../components/lifecycle'

type SheetKind =
  | { kind: 'request' }
  | { kind: 'edit'; request: FiringRequest }
  | { kind: 'decide'; request: FiringRequest; decision: 'approve' | 'decline' }
  | { kind: 'withdraw'; request: FiringRequest }
  | { kind: 'hold' | 'release' | 'reschedule' | 'cancel' | 'fullness' }
  | { kind: 'correct'; field: TimestampField }

export default function FiringPage() {
  const id = Number(useParams().id)
  const { data, error, setData } = useLoad(() => Promise.all([getFiring(id), getOptions()]), [id])
  const [sheet, setSheet] = useState<SheetKind | null>(null)
  const [warnings, setWarnings] = useState<string[]>([])
  const [actionError, setActionError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  // "Request space" on the public page links here with ?request=1 (after login).
  const [params, setParams] = useSearchParams()
  const wantsRequest = params.has('request')
  const detailLoaded = data?.[0]
  useEffect(() => {
    if (!wantsRequest || !detailLoaded) return
    if (detailLoaded.can.request && !detailLoaded.my_request) setSheet({ kind: 'request' })
    setParams({}, { replace: true })
  }, [wantsRequest, detailLoaded, setParams])

  if (error) {
    return (
      <div className="space-y-4">
        <Alert kind="error">{error === 'not found' ? 'That firing does not exist.' : error}</Alert>
        <Link to="/" className="kc-key kc-key--block">
          Home
        </Link>
      </div>
    )
  }
  if (!data) return <PageSpinner />
  const [detail, options] = data
  const { firing: f, can } = detail
  const tz = options.timezone

  /** Runs a mutation, swaps in the fresh detail, and shows warnings/errors. */
  async function run(fn: () => Promise<MutationResult>) {
    setBusy(true)
    setActionError(null)
    try {
      const res = await fn()
      setData([res, options])
      setWarnings(res.warnings)
      setSheet(null)
      window.scrollTo({ top: 0, behavior: 'smooth' })
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }
  const closeSheet = () => {
    setSheet(null)
    setActionError(null)
  }

  const kmControls = can.confirm || can.hold || can.release || can.reschedule || can.cancel || can.manage

  return (
    <div className="space-y-5">
      {/* Header */}
      <section className="space-y-2">
        <Link to="/" className="kc-back">
          ← All firings
        </Link>
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="kc-title">{firingTitle(f)}</h1>
          <StatusBadge status={f.status} />
        </div>
        <p className="kc-data">{formatDate(f.scheduled_date)}</p>
        <p className="kc-caption kc-caption--body">
          <span className="capitalize">{f.firing_type}</span> · Cone {f.cone}
          {kilnLabel(options, f.kiln_id) && ` · ${kilnLabel(options, f.kiln_id)}`}
        </p>
        {f.started_at && (
          <p className="kc-data kc-muted">
            Started {formatWhen(f.started_at, tz)}
            {f.unloaded_at
              ? ` · unloaded ${formatWhen(f.unloaded_at, tz)}`
              : f.estimated_unload_at && ` · est. unload ${formatWhen(f.estimated_unload_at, tz)}`}
          </p>
        )}
        {f.status === 'on_hold' && f.hold_reason && <p className="kc-warning">On hold: {f.hold_reason}</p>}
        {f.status === 'cancelled' && f.cancel_reason && <p className="kc-warning">Cancelled: {f.cancel_reason}</p>}
      </section>

      {warnings.length > 0 && (
        <Alert kind="warning">
          <ul className="space-y-1">
            {warnings.map((w) => (
              <li key={w}>{w}</li>
            ))}
          </ul>
        </Alert>
      )}
      {!sheet && actionError && <Alert kind="error">{actionError}</Alert>}

      {/* Fullness + requests window */}
      {f.status !== 'cancelled' && (
        <section className="kc-plate">
          <header className="kc-plate__head">
            <span className="kc-label">Capacity</span>
          </header>
          <div className="kc-plate__body kc-plate__body--tight">
            <FullnessBar fullness={f.fullness} large />
            <p className="kc-caption kc-caption--body">
              {f.requests_open
                ? `Requests close ${formatWhen(f.requests_close_at, tz)} (${relative(f.requests_close_at)}).`
                : f.requests_closed_manually
                  ? 'Requests closed by a kiln master.'
                  : 'Requests are closed.'}
            </p>
            {f.notes && <p className="kc-caption kc-caption--body whitespace-pre-wrap">{f.notes}</p>}
          </div>
        </section>
      )}

      {/* Actions: lifecycle next steps (same as the kiln QR page) + kiln-master controls */}
      {(detail.can.next_steps.length > 0 || kmControls) && (
        <section className="space-y-4">
          <h2 className="kc-title kc-title--sm">Actions</h2>
          <LifecyclePanel
            detail={detail}
            options={options}
            source="web"
            onDone={(res) => {
              setData([res, options])
              setWarnings(res.warnings)
            }}
          />
          {kmControls && (
            <div className="kc-keys kc-keys--grid">
              {can.confirm && (
                <button disabled={busy} onClick={() => run(() => act.confirm(f.id))} className="kc-key kc-key--go">
                  Confirm firing
                </button>
              )}
              {can.release && (
                <button onClick={() => setSheet({ kind: 'release' })} className="kc-key kc-key--go">
                  Release hold
                </button>
              )}
              {can.reschedule && <KmButton onClick={() => setSheet({ kind: 'reschedule' })}>Reschedule</KmButton>}
              {can.hold && <KmButton onClick={() => setSheet({ kind: 'hold' })}>Put on hold</KmButton>}
              {can.manage && <KmButton onClick={() => setSheet({ kind: 'fullness' })}>Set fullness</KmButton>}
              {can.manage && (
                <KmButton disabled={busy} onClick={() => run(() => act.setRequestsClosed(f.id, f.requests_open))}>
                  {f.requests_open ? 'Close requests' : 'Reopen requests'}
                </KmButton>
              )}
              {can.cancel && (
                <button onClick={() => setSheet({ kind: 'cancel' })} className="kc-key kc-key--block kc-key--danger">
                  Cancel firing
                </button>
              )}
            </div>
          )}
        </section>
      )}

      {/* My request / request space */}
      {detail.my_request ? (
        <MyRequest
          request={detail.my_request}
          onEdit={() => setSheet({ kind: 'edit', request: detail.my_request! })}
          onWithdraw={() => setSheet({ kind: 'withdraw', request: detail.my_request! })}
        />
      ) : (
        can.request && (
          <button onClick={() => setSheet({ kind: 'request' })} className="kc-key kc-key--block kc-key--fire">
            Request space
          </button>
        )
      )}

      {/* Who's in */}
      <section className="space-y-4">
        <h2 className="kc-title kc-title--sm">Who's in</h2>
        <article className="kc-plate kc-plate--pad space-y-2">
          <div className="flex items-baseline justify-between gap-2">
            <span className="kc-label">{f.proposer.name}</span>
            <span className="kc-value">{f.proposer_share_pct}%</span>
          </div>
          <p className="kc-caption kc-caption--body">{f.proposer_description}</p>
          <p className="kc-data kc-muted">
            Proposer · Cone {f.proposer_cone}
            {f.proposer_clay_body && ` · ${f.proposer_clay_body}`}
          </p>
        </article>
        {detail.requests.map((r) => (
          <RequestCard
            key={r.id}
            request={r}
            onDecide={(decision) => setSheet({ kind: 'decide', request: r, decision })}
          />
        ))}
      </section>

      {/* History: program, recorded times and the event log, tucked away */}
      <History detail={detail} tz={tz} onCorrect={(field) => setSheet({ kind: 'correct', field })} />

      <ActionSheets
        sheet={sheet}
        onClose={closeSheet}
        detail={detail}
        options={options}
        busy={busy}
        error={actionError}
        run={run}
      />
    </div>
  )
}

function KmButton({ children, ...props }: { children: ReactNode; onClick: () => void; disabled?: boolean }) {
  return (
    <button {...props} className="kc-key flex-1">
      {children}
    </button>
  )
}

// ── Requests ────────────────────────────────────────────────────────────────

const REQUEST_STATUS: Record<FiringRequest['status'], { className: string; label: string }> = {
  pending: { className: 'kc-tag--flag', label: 'Pending' },
  approved: { className: 'kc-tag--go', label: 'Approved' },
  declined: { className: 'kc-tag--stop', label: 'Declined' },
  withdrawn: { className: 'kc-tag--void', label: 'Withdrawn' },
}

function RequestStatusTag({ status }: { status: FiringRequest['status'] }) {
  const s = REQUEST_STATUS[status]
  return <span className={`kc-tag ${s.className}`}>{s.label}</span>
}

function RequestDetails({ r }: { r: FiringRequest }) {
  return (
    <>
      <p className="kc-caption kc-caption--body">{r.description}</p>
      <p className="kc-data kc-muted">
        Cone {r.cone}
        {r.glazed && ' · glazed'}
        {r.clay_body && ` · ${r.clay_body}`}
      </p>
      {r.compatibility_warning && <p className="kc-warning">⚠ {r.compatibility_warning}</p>}
      {r.decision_note && <p className="kc-caption kc-caption--body">Kiln master: “{r.decision_note}”</p>}
    </>
  )
}

function MyRequest({ request: r, onEdit, onWithdraw }: { request: FiringRequest; onEdit: () => void; onWithdraw: () => void }) {
  return (
    <section className="kc-plate kc-plate--accent">
      <header className="kc-plate__head">
        <span className="kc-label">Your request</span>
        <RequestStatusTag status={r.status} />
      </header>
      <div className="kc-plate__body kc-plate__body--tight">
        <p className="kc-value">{r.estimated_share_pct}%</p>
        <RequestDetails r={r} />
        {(r.can.edit || r.can.withdraw) && (
          <div className="kc-keys kc-keys--fill pt-2">
            {r.can.edit && (
              <button onClick={onEdit} className="kc-key">
                Edit
              </button>
            )}
            {r.can.withdraw && (
              <button onClick={onWithdraw} className="kc-key">
                Withdraw
              </button>
            )}
          </div>
        )}
      </div>
    </section>
  )
}

function RequestCard({ request: r, onDecide }: { request: FiringRequest; onDecide: (d: 'approve' | 'decline') => void }) {
  const inactive = r.status === 'declined' || r.status === 'withdrawn'
  return (
    <article className={`kc-plate${inactive ? ' opacity-60' : ''}`}>
      <header className="kc-plate__head">
        <span className="kc-request__who min-w-0">
          {r.user_avatar_url && <img src={r.user_avatar_url} alt="" className="kc-avatar size-7" />}
          <span className="truncate">{r.user.name}</span>
          <RequestStatusTag status={r.status} />
        </span>
        <span className="kc-value">{r.estimated_share_pct}%</span>
      </header>
      <div className="kc-plate__body kc-plate__body--tight">
        <RequestDetails r={r} />
        {r.can.decide && (
          <div className="kc-keys kc-keys--fill pt-2">
            {r.status !== 'approved' && (
              <button onClick={() => onDecide('approve')} className="kc-key kc-key--go">
                Approve
              </button>
            )}
            {r.status !== 'declined' && (
              <button onClick={() => onDecide('decline')} className="kc-key kc-key--danger">
                Decline
              </button>
            )}
          </div>
        )}
      </div>
    </article>
  )
}

function RequestForm({
  initial,
  firing,
  options,
  busy,
  submitLabel,
  onSubmit,
}: {
  initial: RequestInput
  firing: FiringDetail['firing']
  options: Options
  busy: boolean
  submitLabel: string
  onSubmit: (r: RequestInput) => void
}) {
  const [r, setR] = useState(initial)
  const set = <K extends keyof RequestInput>(k: K, v: RequestInput[K]) => setR((p) => ({ ...p, [k]: v }))

  // Live compatibility hints (§7); the server records the same warning for the kiln master.
  const hints: string[] = []
  if (r.cone !== firing.cone) hints.push(`This is a cone ${firing.cone} firing.`)
  if (r.glazed && firing.firing_type.toLowerCase() === 'bisque') hints.push('This is a bisque firing; glazed work usually goes in a glaze firing.')
  if (firing.fullness.calculated_pct + r.share_pct > 100) hints.push('This may not fit. The kiln master will decide.')

  return (
    <form
      onSubmit={(e: FormEvent) => {
        e.preventDefault()
        onSubmit({ ...r, clay_body: r.clay_body || null })
      }}
      className="space-y-4"
    >
      <Field label="What are you putting in?">
        <textarea
          required
          rows={2}
          maxLength={1000}
          value={r.description}
          onChange={(e) => set('description', e.target.value)}
          placeholder="4 mugs, 2 medium bowls"
          className="kc-textarea"
        />
      </Field>
      <ShareInput value={r.share_pct} onChange={(v) => set('share_pct', v)} kiln={options.kilns.find((k) => k.id === firing.kiln_id)} />
      <ConeSelect value={r.cone} onChange={(v) => set('cone', v)} cones={options.cones} label="Cone your work is for" />
      <label className="kc-check">
        <input type="checkbox" checked={r.glazed} onChange={(e) => set('glazed', e.target.checked)} className="kc-checkbox" />
        My work is glazed
      </label>
      <Field label="Clay body (optional)">
        <input value={r.clay_body ?? ''} maxLength={120} onChange={(e) => set('clay_body', e.target.value)} className={inputClass} />
      </Field>
      {hints.length > 0 && (
        <Alert kind="warning">
          {hints.map((h) => (
            <p key={h}>{h}</p>
          ))}
        </Alert>
      )}
      <button type="submit" disabled={busy} className="kc-key kc-key--block kc-key--fire">
        {busy ? 'Saving…' : submitLabel}
      </button>
    </form>
  )
}

// ── Sheets ──────────────────────────────────────────────────────────────────

function ActionSheets({
  sheet,
  onClose,
  detail,
  options,
  busy,
  error,
  run,
}: {
  sheet: SheetKind | null
  onClose: () => void
  detail: FiringDetail
  options: Options
  busy: boolean
  error: string | null
  run: (fn: () => Promise<MutationResult>) => Promise<void>
}) {
  const f = detail.firing
  const tz = options.timezone
  const err = error && <Alert kind="error">{error}</Alert>

  let title = ''
  let body: ReactNode = null
  switch (sheet?.kind) {
    case 'request':
      title = 'Request space'
      body = (
        <RequestForm
          initial={{ description: '', share_pct: 10, cone: f.cone, clay_body: null, glazed: f.firing_type !== 'bisque' }}
          firing={f}
          options={options}
          busy={busy}
          submitLabel="Send request"
          onSubmit={(r) => run(() => act.createRequest(f.id, r))}
        />
      )
      break
    case 'edit': {
      const r = sheet.request
      title = 'Edit your request'
      body = (
        <>
          {r.status === 'approved' && <Alert kind="info">Your request is approved. Editing it sends it back for approval.</Alert>}
          <RequestForm
            initial={{ description: r.description, share_pct: r.estimated_share_pct, cone: r.cone, clay_body: r.clay_body, glazed: r.glazed }}
            firing={f}
            options={options}
            busy={busy}
            submitLabel="Save changes"
            onSubmit={(input) => run(() => act.updateRequest(f.id, r.id, input))}
          />
        </>
      )
      break
    }
    case 'withdraw':
      title = 'Withdraw your request?'
      body = (
        <ConfirmBody busy={busy} label="Withdraw" danger onConfirm={() => run(() => act.withdrawRequest(f.id, sheet.request.id))}>
          <p>Your work won't be in this firing. You can request again while requests are open.</p>
        </ConfirmBody>
      )
      break
    case 'decide': {
      const { request: r, decision } = sheet
      title = `${decision === 'approve' ? 'Approve' : 'Decline'} ${r.user.name}'s request`
      body = (
        <ReasonForm
          label="Note to the requester (optional)"
          required={false}
          busy={busy}
          submitLabel={decision === 'approve' ? `Approve ${r.estimated_share_pct}%` : 'Decline'}
          danger={decision === 'decline'}
          onSubmit={(note) => run(() => act.decide(f.id, r.id, decision, note || null))}
        >
          <p className="kc-caption kc-caption--body">
            {r.estimated_share_pct}% · {r.description}
          </p>
          {r.compatibility_warning && <Alert kind="warning">{r.compatibility_warning}</Alert>}
        </ReasonForm>
      )
      break
    }
    case 'hold':
      title = 'Put on hold'
      body = (
        <ReasonForm label="Why? (shown to everyone)" required busy={busy} submitLabel="Put on hold" onSubmit={(reason) => run(() => act.hold(f.id, reason))}>
          <p className="kc-caption kc-caption--body">Requests stay open unless you close them.</p>
        </ReasonForm>
      )
      break
    case 'cancel':
      title = 'Cancel this firing?'
      body = (
        <ReasonForm label="Why? (shown to everyone)" required danger busy={busy} submitLabel="Cancel firing" onSubmit={(reason) => run(() => act.cancel(f.id, reason))}>
          <p className="kc-caption kc-caption--body">Everyone in the firing will be told. This can't be undone.</p>
        </ReasonForm>
      )
      break
    case 'reschedule':
      title = 'Reschedule'
      body = (
        <DayForm
          initial={f.scheduled_date}
          busy={busy}
          withNote
          submitLabel="Reschedule"
          onSubmit={(day, note) => run(() => act.reschedule(f.id, day, note))}
        />
      )
      break
    case 'release':
      title = 'Release hold'
      body = (
        <DayForm
          initial={f.scheduled_date}
          busy={busy}
          submitLabel="Release"
          onSubmit={(day) => run(() => act.release(f.id, day))}
        >
          <p className="kc-caption kc-caption--body">Change the day if needed.</p>
        </DayForm>
      )
      break
    case 'fullness':
      title = 'Set fullness'
      body = <FullnessForm detail={detail} busy={busy} onSubmit={(pct) => run(() => act.setFullness(f.id, pct))} />
      break
    case 'correct':
      title = 'Correct time'
      body = (
        <CorrectTimeForm
          detail={detail}
          field={sheet.field}
          tz={tz}
          busy={busy}
          onSubmit={(iso) => run(() => act.correctTimestamp(f.id, sheet.field, iso))}
        />
      )
      break
  }

  return (
    <Sheet title={title} open={!!sheet} onClose={onClose}>
      {body}
      {err}
    </Sheet>
  )
}

function ConfirmBody({ children, busy, label, danger, onConfirm }: { children: ReactNode; busy: boolean; label: string; danger?: boolean; onConfirm: () => void }) {
  return (
    <>
      {children}
      <button disabled={busy} onClick={onConfirm} className={`kc-key kc-key--block ${danger ? 'kc-key--fire' : ''}`}>
        {busy ? 'Working…' : label}
      </button>
    </>
  )
}

function ReasonForm({
  children,
  label,
  required,
  busy,
  submitLabel,
  danger,
  onSubmit,
}: {
  children?: ReactNode
  label: string
  required: boolean
  busy: boolean
  submitLabel: string
  danger?: boolean
  onSubmit: (text: string) => void
}) {
  const [text, setText] = useState('')
  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit(text.trim())
      }}
    >
      {children}
      <Field label={label}>
        <textarea rows={2} required={required} maxLength={500} value={text} onChange={(e) => setText(e.target.value)} className="kc-textarea" />
      </Field>
      <button type="submit" disabled={busy} className={`kc-key kc-key--block ${danger ? 'kc-key--fire' : ''}`}>
        {busy ? 'Working…' : submitLabel}
      </button>
    </form>
  )
}

function DayForm({
  children,
  initial,
  busy,
  withNote,
  submitLabel,
  onSubmit,
}: {
  children?: ReactNode
  initial: string
  busy: boolean
  withNote?: boolean
  submitLabel: string
  onSubmit: (day: string, note: string | null) => void
}) {
  const [value, setValue] = useState(initial)
  const [note, setNote] = useState('')
  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit(value, note.trim() || null)
      }}
    >
      {children}
      <Field
        label="Day"
        hint={
          <span className="kc-caption kc-caption--body block">
            Firings are booked for a day; no start time is promised. Requests close moves with the day.
          </span>
        }
      >
        <input type="date" required value={value} onChange={(e) => setValue(e.target.value)} className={inputClass} />
      </Field>
      {withNote && (
        <Field label="Note (optional)">
          <input value={note} maxLength={500} onChange={(e) => setNote(e.target.value)} className={inputClass} />
        </Field>
      )}
      <button type="submit" disabled={busy} className="kc-key kc-key--block kc-key--fire">
        {busy ? 'Working…' : submitLabel}
      </button>
    </form>
  )
}

function FullnessForm({ detail, busy, onSubmit }: { detail: FiringDetail; busy: boolean; onSubmit: (pct: number | null) => void }) {
  const full = detail.firing.fullness
  const [pct, setPct] = useState(full.overridden ? full.display_pct : Math.min(full.calculated_pct, 100))
  return (
    <div className="space-y-4">
      <p className="kc-caption kc-caption--body">
        Calculated from approved requests: <strong>{full.calculated_pct}%</strong>. Set what it really looks like.
      </p>
      <div className="flex items-baseline justify-between gap-3">
        <span className="kc-field__label kc-field__label--flush">Fullness</span>
        <span className="kc-value">{pct}%</span>
      </div>
      <input type="range" min={0} max={100} value={pct} onChange={(e) => setPct(Number(e.target.value))} className="kc-range" aria-label="Fullness percent" />
      <button disabled={busy} onClick={() => onSubmit(pct)} className="kc-key kc-key--block kc-key--fire">
        Set to {pct}%
      </button>
      {full.overridden && (
        <button disabled={busy} onClick={() => onSubmit(null)} className="kc-key kc-key--block">
          Use calculated ({Math.min(full.calculated_pct, 100)}%)
        </button>
      )}
    </div>
  )
}

// ── Timeline ────────────────────────────────────────────────────────────────

function describe(e: FiringEvent): string {
  switch (e.type) {
    case 'created':
      return 'Proposed the firing'
    case 'status_change':
      return `${e.from_status ? STATUS_LABELS[e.from_status] : '?'} → ${e.to_status ? STATUS_LABELS[e.to_status] : '?'}`
    case 'requests_closed':
      return e.source === 'system' ? 'Requests closed' : 'Closed requests'
    case 'requests_reopened':
      return 'Reopened requests'
    default:
      return ''
  }
}

/** Collapsed audit trail: program, recorded times (fixable by kiln masters), and events. */
function History({ detail, tz, onCorrect }: { detail: FiringDetail; tz: string; onCorrect: (field: TimestampField) => void }) {
  const f = detail.firing
  const rows = TIME_ROWS.filter((r) => f[r.field])
  if (detail.events.length === 0 && !f.program && rows.length === 0) return null
  return (
    <details>
      <summary className="kc-title kc-title--sm cursor-pointer">History</summary>
      <div className="mt-4 space-y-4">
        {f.program && (
          <p className="kc-data">
            Program: {f.program}
            {f.estimated_duration_minutes && ` · about ${Math.round((f.estimated_duration_minutes / 60) * 10) / 10}h`}
          </p>
        )}
        {rows.length > 0 && (
          <div>
            {rows.map((r) => (
              <div key={r.field} className="kc-spec-row">
                <span>
                  <span className="kc-label kc-muted">{r.label}:</span> <span className="kc-data">{formatWhen(f[r.field]!, tz)}</span>
                </span>
                {detail.can.correct_timestamps && (
                  <button onClick={() => onCorrect(r.field)} className="kc-key kc-key--sm">
                    Fix
                  </button>
                )}
              </div>
            ))}
          </div>
        )}
        {detail.events.length > 0 && (
          <ol className="kc-log kc-log--stack">
            {[...detail.events].reverse().map((e) => {
              const what = describe(e)
              return (
                <li key={e.id}>
                  <span className="kc-log__time">
                    {formatWhen(e.occurred_at, tz)}
                    {e.actor && ` · ${e.actor.name}`}
                    {e.source === 'kiln_qr' && ' · kiln QR'}
                    {e.source === 'system' && ' · automatic'}
                  </span>
                  {what && <p className="kc-label">{what}</p>}
                  {e.note && <p className={what ? 'kc-caption kc-caption--body' : 'kc-label'}>{e.note}</p>}
                </li>
              )
            })}
          </ol>
        )}
      </div>
    </details>
  )
}
