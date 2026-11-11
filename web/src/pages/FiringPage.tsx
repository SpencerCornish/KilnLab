import { useState, type FormEvent, type ReactNode } from 'react'
import { Link, useParams } from 'react-router'
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
import { firingActions as act, getFiring, getOptions, listLoaders } from '../lib/api'
import { PageSpinner } from '../lib/session'
import { formatWhen, fromInputValue, relative, toInputValue } from '../lib/time'
import type {
  FiringDetail,
  FiringEvent,
  FiringRequest,
  LoaderCandidate,
  MutationResult,
  Options,
  RequestInput,
  TimestampField,
} from '../lib/types'
import { useLoad } from '../lib/useLoad'
import { CorrectTimeForm, LifecyclePanel, RecordedTimes } from '../components/lifecycle'

type SheetKind =
  | { kind: 'request' }
  | { kind: 'edit'; request: FiringRequest }
  | { kind: 'decide'; request: FiringRequest; decision: 'approve' | 'decline' }
  | { kind: 'withdraw'; request: FiringRequest }
  | { kind: 'hold' | 'release' | 'reschedule' | 'cancel' | 'fullness' | 'loader' }
  | { kind: 'correct'; field: TimestampField }

export default function FiringPage() {
  const id = Number(useParams().id)
  const { data, error, setData } = useLoad(() => Promise.all([getFiring(id), getOptions()]), [id])
  const [sheet, setSheet] = useState<SheetKind | null>(null)
  const [warnings, setWarnings] = useState<string[]>([])
  const [actionError, setActionError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  if (error) {
    return (
      <div className="space-y-4">
        <Alert kind="error">{error === 'not found' ? 'That firing does not exist.' : error}</Alert>
        <Link to="/" className="btn border-2 border-stone-300 bg-white">
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
        <Link to="/" className="text-sm font-semibold text-stone-500">
          ← All firings
        </Link>
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-2xl font-bold">{firingTitle(f)}</h1>
          <StatusBadge status={f.status} />
        </div>
        <p className="text-lg">{formatWhen(f.scheduled_start, tz)}</p>
        <p className="text-stone-600">
          <span className="capitalize">{f.firing_type}</span> · Cone {f.cone}
          {kilnLabel(options, f.kiln_id) && ` · ${kilnLabel(options, f.kiln_id)}`} · proposed by {f.proposer.name}
          {detail.loader && <> · loader: {detail.loader.name}</>}
        </p>
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
      {f.status === 'on_hold' && f.hold_reason && <Alert kind="warning">On hold: {f.hold_reason}</Alert>}
      {f.status === 'cancelled' && f.cancel_reason && <Alert kind="error">Cancelled: {f.cancel_reason}</Alert>}

      {/* Fullness + requests window */}
      {f.status !== 'cancelled' && (
        <section className="card space-y-3">
          <FullnessBar fullness={f.fullness} large />
          <p className="text-sm text-stone-600">
            {f.requests_open
              ? `Requests close ${formatWhen(f.requests_close_at, tz)} (${relative(f.requests_close_at)}).`
              : f.requests_closed_manually
                ? 'Requests closed by a kiln master.'
                : 'Requests are closed.'}
          </p>
          {f.notes && <p className="whitespace-pre-wrap border-t border-stone-200 pt-3">{f.notes}</p>}
        </section>
      )}

      {/* Lifecycle: next steps (same as the kiln QR page) + recorded times */}
      {detail.can.next_steps.length > 0 && (
        <section className="space-y-3">
          <h2 className="text-lg font-bold">Update status</h2>
          <LifecyclePanel
            detail={detail}
            options={options}
            source="web"
            onDone={(res) => {
              setData([res, options])
              setWarnings(res.warnings)
            }}
          />
        </section>
      )}
      <RecordedTimes detail={detail} tz={tz} onCorrect={(field) => setSheet({ kind: 'correct', field })} />

      {/* My request / request space */}
      {detail.my_request ? (
        <MyRequest
          request={detail.my_request}
          onEdit={() => setSheet({ kind: 'edit', request: detail.my_request! })}
          onWithdraw={() => setSheet({ kind: 'withdraw', request: detail.my_request! })}
        />
      ) : (
        can.request && (
          <button onClick={() => setSheet({ kind: 'request' })} className="btn bg-orange-600 text-white">
            Request space
          </button>
        )
      )}

      {/* Who's in */}
      <section className="space-y-3">
        <h2 className="text-lg font-bold">Who's in</h2>
        <div className="card space-y-1">
          <div className="flex items-baseline justify-between gap-2">
            <span className="font-semibold">{f.proposer.name}</span>
            <span className="font-bold tabular-nums">{f.proposer_share_pct}%</span>
          </div>
          <p className="text-stone-700">{f.proposer_description}</p>
          <p className="text-sm text-stone-500">
            Proposer · Cone {f.proposer_cone}
            {f.proposer_clay_body && ` · ${f.proposer_clay_body}`}
          </p>
        </div>
        {detail.requests.map((r) => (
          <RequestCard
            key={r.id}
            request={r}
            onDecide={(decision) => setSheet({ kind: 'decide', request: r, decision })}
          />
        ))}
      </section>

      {/* Kiln master controls */}
      {kmControls && (
        <section className="space-y-3">
          <h2 className="text-lg font-bold">Kiln master</h2>
          <div className="grid grid-cols-2 gap-3">
            {can.confirm && (
              <button disabled={busy} onClick={() => run(() => act.confirm(f.id))} className="btn col-span-2 bg-green-700 text-white">
                Confirm firing
              </button>
            )}
            {can.release && (
              <button onClick={() => setSheet({ kind: 'release' })} className="btn col-span-2 bg-green-700 text-white">
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
            {can.manage && <KmButton onClick={() => setSheet({ kind: 'loader' })}>Assign loader</KmButton>}
            {can.cancel && (
              <button onClick={() => setSheet({ kind: 'cancel' })} className="btn border-2 border-red-300 bg-white text-base text-red-700">
                Cancel firing
              </button>
            )}
          </div>
        </section>
      )}

      <Timeline events={detail.events} tz={tz} />

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
    <button {...props} className="btn border-2 border-stone-900 bg-white text-base">
      {children}
    </button>
  )
}

// ── Requests ────────────────────────────────────────────────────────────────

const REQUEST_STATUS_STYLES: Record<FiringRequest['status'], string> = {
  pending: 'bg-amber-100 text-amber-900',
  approved: 'bg-green-100 text-green-900',
  declined: 'bg-red-100 text-red-900',
  withdrawn: 'bg-stone-200 text-stone-600',
}

function RequestStatusChip({ status }: { status: FiringRequest['status'] }) {
  return <span className={`rounded-full px-2 py-0.5 text-sm font-semibold capitalize ${REQUEST_STATUS_STYLES[status]}`}>{status}</span>
}

function RequestDetails({ r }: { r: FiringRequest }) {
  return (
    <>
      <p className="text-stone-700">{r.description}</p>
      <p className="text-sm text-stone-500">
        Cone {r.cone}
        {r.glazed && ' · glazed'}
        {r.clay_body && ` · ${r.clay_body}`}
      </p>
      {r.compatibility_warning && <p className="text-sm font-semibold text-amber-800">⚠ {r.compatibility_warning}</p>}
      {r.decision_note && <p className="text-sm text-stone-600">Kiln master: “{r.decision_note}”</p>}
    </>
  )
}

function MyRequest({ request: r, onEdit, onWithdraw }: { request: FiringRequest; onEdit: () => void; onWithdraw: () => void }) {
  return (
    <section className="card space-y-2 border-2 border-orange-300">
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-lg font-bold">Your request</h2>
        <RequestStatusChip status={r.status} />
      </div>
      <p className="text-2xl font-bold tabular-nums">{r.estimated_share_pct}%</p>
      <RequestDetails r={r} />
      {(r.can.edit || r.can.withdraw) && (
        <div className="grid grid-cols-2 gap-3 pt-2">
          {r.can.edit && (
            <button onClick={onEdit} className="btn border-2 border-stone-900 bg-white text-base">
              Edit
            </button>
          )}
          {r.can.withdraw && (
            <button onClick={onWithdraw} className="btn border-2 border-stone-300 bg-white text-base text-stone-700">
              Withdraw
            </button>
          )}
        </div>
      )}
    </section>
  )
}

function RequestCard({ request: r, onDecide }: { request: FiringRequest; onDecide: (d: 'approve' | 'decline') => void }) {
  return (
    <div className={`card space-y-1 ${r.status === 'declined' || r.status === 'withdrawn' ? 'opacity-60' : ''}`}>
      <div className="flex items-center justify-between gap-2">
        <span className="flex min-w-0 items-center gap-2">
          {r.user_avatar_url && <img src={r.user_avatar_url} alt="" className="size-7 rounded-full" />}
          <span className="truncate font-semibold">{r.user.name}</span>
          <RequestStatusChip status={r.status} />
        </span>
        <span className="font-bold tabular-nums">{r.estimated_share_pct}%</span>
      </div>
      <RequestDetails r={r} />
      {r.can.decide && (
        <div className="grid grid-cols-2 gap-3 pt-2">
          {r.status !== 'approved' && (
            <button onClick={() => onDecide('approve')} className="btn bg-green-700 text-base text-white">
              Approve
            </button>
          )}
          {r.status !== 'declined' && (
            <button onClick={() => onDecide('decline')} className="btn border-2 border-red-300 bg-white text-base text-red-700">
              Decline
            </button>
          )}
        </div>
      )}
    </div>
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
          className={inputClass}
        />
      </Field>
      <ShareInput value={r.share_pct} onChange={(v) => set('share_pct', v)} kiln={options.kilns.find((k) => k.id === firing.kiln_id)} />
      <ConeSelect value={r.cone} onChange={(v) => set('cone', v)} cones={options.cones} label="Cone your work is for" />
      <label className="flex min-h-12 items-center gap-3 text-lg">
        <input type="checkbox" checked={r.glazed} onChange={(e) => set('glazed', e.target.checked)} className="size-6 accent-orange-600" />
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
      <button type="submit" disabled={busy} className="btn bg-orange-600 text-white">
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
          <p className="text-stone-700">
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
          <p className="text-stone-600">Requests stay open unless you close them.</p>
        </ReasonForm>
      )
      break
    case 'cancel':
      title = 'Cancel this firing?'
      body = (
        <ReasonForm label="Why? (shown to everyone)" required danger busy={busy} submitLabel="Cancel firing" onSubmit={(reason) => run(() => act.cancel(f.id, reason))}>
          <p className="text-stone-600">Everyone in the firing will be told. This can't be undone.</p>
        </ReasonForm>
      )
      break
    case 'reschedule':
      title = 'Reschedule'
      body = (
        <DateForm
          initial={toInputValue(f.scheduled_start, tz)}
          tz={tz}
          busy={busy}
          withNote
          submitLabel="Reschedule"
          onSubmit={(iso, note) => iso && run(() => act.reschedule(f.id, iso, note))}
        />
      )
      break
    case 'release':
      title = 'Release hold'
      body = (
        <DateForm
          initial={toInputValue(f.scheduled_start, tz)}
          tz={tz}
          busy={busy}
          submitLabel="Release"
          onSubmit={(iso) => run(() => act.release(f.id, iso))}
        >
          <p className="text-stone-600">Change the start time if needed.</p>
        </DateForm>
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
    case 'loader':
      title = 'Assign loader'
      body = <LoaderForm current={f.loader_user_id} busy={busy} onSubmit={(uid) => run(() => act.setLoader(f.id, uid))} />
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
      <button disabled={busy} onClick={onConfirm} className={`btn text-white ${danger ? 'bg-red-700' : 'bg-orange-600'}`}>
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
        <textarea rows={2} required={required} maxLength={500} value={text} onChange={(e) => setText(e.target.value)} className={inputClass} />
      </Field>
      <button type="submit" disabled={busy} className={`btn text-white ${danger ? 'bg-red-700' : 'bg-orange-600'}`}>
        {busy ? 'Working…' : submitLabel}
      </button>
    </form>
  )
}

function DateForm({
  children,
  initial,
  tz,
  busy,
  withNote,
  submitLabel,
  onSubmit,
}: {
  children?: ReactNode
  initial: string
  tz: string
  busy: boolean
  withNote?: boolean
  submitLabel: string
  onSubmit: (iso: string | null, note: string | null) => void
}) {
  const [value, setValue] = useState(initial)
  const [note, setNote] = useState('')
  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit(fromInputValue(value, tz), note.trim() || null)
      }}
    >
      {children}
      <Field label="Start" hint={<p className="mt-1 text-sm text-stone-500">{tz.replace('_', ' ')} time. Requests close moves with it.</p>}>
        <input type="datetime-local" required value={value} onChange={(e) => setValue(e.target.value)} className={inputClass} />
      </Field>
      {withNote && (
        <Field label="Note (optional)">
          <input value={note} maxLength={500} onChange={(e) => setNote(e.target.value)} className={inputClass} />
        </Field>
      )}
      <button type="submit" disabled={busy} className="btn bg-orange-600 text-white">
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
      <p className="text-stone-600">
        Calculated from approved requests: <strong>{full.calculated_pct}%</strong>. Set what it really looks like.
      </p>
      <div className="flex items-baseline justify-between">
        <span className="font-semibold">Fullness</span>
        <span className="text-2xl font-bold tabular-nums">{pct}%</span>
      </div>
      <input type="range" min={0} max={100} value={pct} onChange={(e) => setPct(Number(e.target.value))} className="h-10 w-full accent-orange-600" aria-label="Fullness percent" />
      <button disabled={busy} onClick={() => onSubmit(pct)} className="btn bg-orange-600 text-white">
        Set to {pct}%
      </button>
      {full.overridden && (
        <button disabled={busy} onClick={() => onSubmit(null)} className="btn border-2 border-stone-300 bg-white text-base">
          Use calculated ({Math.min(full.calculated_pct, 100)}%)
        </button>
      )}
    </div>
  )
}

function LoaderForm({ current, busy, onSubmit }: { current: number | null; busy: boolean; onSubmit: (id: number | null) => void }) {
  const { data, error } = useLoad(listLoaders, [])
  if (error) return <Alert kind="error">{error}</Alert>
  if (!data) return <PageSpinner />
  return (
    <div className="space-y-2">
      {data.map((c: LoaderCandidate) => (
        <button
          key={c.id}
          disabled={busy}
          onClick={() => onSubmit(c.id)}
          className={`btn justify-between border-2 text-base ${c.id === current ? 'border-orange-600 bg-orange-50' : 'border-stone-300 bg-white'}`}
        >
          <span>{c.name}</span>
          <span className="text-sm font-normal text-stone-500">{c.is_kiln_master ? 'Kiln master' : 'Self-loader'}</span>
        </button>
      ))}
      {data.length === 0 && <p className="text-stone-600">No self-loaders or kiln masters yet.</p>}
      {current !== null && (
        <button disabled={busy} onClick={() => onSubmit(null)} className="btn text-base text-stone-600">
          Unassign
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
      return 'Closed requests'
    case 'requests_reopened':
      return 'Reopened requests'
    default:
      return ''
  }
}

function Timeline({ events, tz }: { events: FiringEvent[]; tz: string }) {
  if (events.length === 0) return null
  return (
    <section className="space-y-3">
      <h2 className="text-lg font-bold">Timeline</h2>
      <ol className="space-y-3 border-l-2 border-stone-300 pl-4">
        {[...events].reverse().map((e) => {
          const what = describe(e)
          return (
            <li key={e.id} className="relative">
              <span className="absolute -left-[1.4rem] top-1.5 size-3 rounded-full bg-stone-400" />
              <p className="text-sm text-stone-500">
                {formatWhen(e.occurred_at, tz)}
                {e.actor && ` · ${e.actor.name}`}
                {e.source === 'kiln_qr' && ' · kiln QR'}
              </p>
              {what && <p className="font-medium">{what}</p>}
              {e.note && <p className={what ? 'text-stone-600' : 'font-medium'}>{e.note}</p>}
            </li>
          )
        })}
      </ol>
    </section>
  )
}
