import { useState, type FormEvent, type ReactNode } from 'react'
import { Link, Navigate } from 'react-router'
import { renderSVG } from 'uqr'
import { Alert, Field, smallInputClass } from '../components/firing'
import { createKiln, getSettings, updateKiln, updateSettings } from '../lib/api'
import { PageSpinner, useSession } from '../lib/session'
import type { AdminSettings, Kiln, KilnInput, Settings } from '../lib/types'
import { useLoad } from '../lib/useLoad'

/** /admin/settings (§13): every studio setting, plus kilns. Admins only. */
export function AdminSettingsPage() {
  const { me } = useSession()
  const { data, error, setData } = useLoad(getSettings, [])
  if (!me?.roles.admin) return <Navigate to="/admin" replace />
  if (error) return <Alert kind="error">{error}</Alert>
  if (!data) return <PageSpinner />
  return (
    <div className="space-y-8">
      <div className="flex items-center justify-between gap-3">
        <h1 className="kc-title">Studio settings</h1>
        <Link to="/admin" className="kc-key kc-key--sm shrink-0">
          Admin
        </Link>
      </div>
      <SettingsForm data={data} onSaved={setData} />
      <KilnsSection kilns={data.kilns} onChanged={(kilns) => setData({ ...data, kilns })} />
      <Link to="/admin/qr" className="kc-key kc-key--block">
        Printable QR codes
      </Link>
    </div>
  )
}

// ── Settings form ───────────────────────────────────────────────────────────

interface ProgramRow {
  name: string
  hours: string
}

function SettingsForm({ data, onSaved }: { data: AdminSettings; onSaved: (d: AdminSettings) => void }) {
  const [s, setS] = useState<Settings>(data.settings)
  const [types, setTypes] = useState(data.settings.firing_types.join(', '))
  const [cones, setCones] = useState(data.settings.cones.join(', '))
  const [programs, setPrograms] = useState<ProgramRow[]>(
    data.settings.programs.map((p) => ({
      name: p.name,
      hours: p.default_duration_minutes ? String(+(p.default_duration_minutes / 60).toFixed(2)) : '',
    })),
  )
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState<{ kind: 'error' | 'info'; text: string } | null>(null)

  const num = (k: keyof Settings) => (e: { target: { value: string } }) => setS({ ...s, [k]: Number(e.target.value) })
  const split = (v: string) => v.split(',').map((x) => x.trim()).filter(Boolean)

  async function save(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setMsg(null)
    try {
      const out = await updateSettings({
        ...s,
        firing_types: split(types),
        cones: split(cones),
        programs: programs
          .filter((p) => p.name.trim())
          .map((p) => {
            const h = parseFloat(p.hours)
            return { name: p.name.trim(), default_duration_minutes: h > 0 ? Math.round(h * 60) : undefined }
          }),
      })
      onSaved(out)
      setMsg({ kind: 'info', text: 'Saved.' })
    } catch (err) {
      setMsg({ kind: 'error', text: err instanceof Error ? err.message : String(err) })
    } finally {
      setBusy(false)
    }
  }

  const env = data.env
  return (
    <form onSubmit={save} className="space-y-4">
      <Group title="Scheduling" open>
        <div className="grid grid-cols-2 items-end gap-3">
          <Field label="Minimum notice (hours)">
            <input type="number" min={0} className={smallInputClass} value={s.min_notice_hours} onChange={num('min_notice_hours')} />
          </Field>
          <Field label="Requests close (hours before start)">
            <input type="number" min={0} className={smallInputClass} value={s.request_cutoff_hours} onChange={num('request_cutoff_hours')} />
          </Field>
        </div>
        <p className="kc-caption kc-caption--body">The request cutoff applies to firings proposed or rescheduled from now on.</p>
        <Field label="Timezone" hint={<span className="kc-caption kc-caption--body block">All times are shown in this zone.</span>}>
          <input className={smallInputClass} value={s.timezone} onChange={(e) => setS({ ...s, timezone: e.target.value })} />
        </Field>
        <Field label="Public page shows the next (days)">
          <input type="number" min={1} className={smallInputClass} value={s.public_page_days_ahead} onChange={num('public_page_days_ahead')} />
        </Field>
      </Group>

      <Group title="Kiln master alerts">
        <Field
          label="Under-filled below (%)"
          hint={<span className="kc-caption kc-caption--body block">Kiln masters get a DM when requests close below this.</span>}
        >
          <input
            type="number"
            min={0}
            max={100}
            className={smallInputClass}
            value={s.underfilled_threshold_pct}
            onChange={num('underfilled_threshold_pct')}
          />
        </Field>
        <div className="grid grid-cols-2 items-end gap-3">
          <Field label="Missed start after (min)">
            <input
              type="number"
              min={0}
              className={smallInputClass}
              value={s.missed_start_grace_minutes}
              onChange={num('missed_start_grace_minutes')}
            />
          </Field>
          <Field label="Missed unload after (min)">
            <input
              type="number"
              min={0}
              className={smallInputClass}
              value={s.missed_unload_grace_minutes}
              onChange={num('missed_unload_grace_minutes')}
            />
          </Field>
        </div>
        <p className="kc-caption kc-caption--body">
          Reminders go out once a firing is this late being marked started, or this long past its estimated unload.
        </p>
        <Field
          label="Day-of reminder (hour)"
          hint={<span className="kc-caption kc-caption--body block">Kiln masters are reminded at this hour, studio time, on the day of each firing.</span>}
        >
          <input type="number" min={0} max={23} className={smallInputClass} value={s.day_reminder_hour} onChange={num('day_reminder_hour')} />
        </Field>
      </Group>

      <Group title="Firing options">
        <Field label="Firing types" hint={<span className="kc-caption kc-caption--body block">Comma-separated.</span>}>
          <input className={smallInputClass} value={types} onChange={(e) => setTypes(e.target.value)} />
        </Field>
        <Field label="Cones" hint={<span className="kc-caption kc-caption--body block">Comma-separated, in the order to show them.</span>}>
          <input className={smallInputClass} value={cones} onChange={(e) => setCones(e.target.value)} />
        </Field>
        <div className="space-y-2">
          <span className="kc-field__label">Controller programs</span>
          <p className="kc-caption kc-caption--body">Offered when starting a firing. The duration fills in the estimate.</p>
          {programs.map((p, i) => (
            <div key={i} className="flex items-center gap-2">
              <input
                aria-label="Program name"
                placeholder="Name"
                className={`${smallInputClass} min-w-0 flex-1`}
                value={p.name}
                onChange={(e) => setPrograms(programs.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))}
              />
              <div className="w-20 shrink-0">
                <input
                  aria-label="Typical hours"
                  placeholder="Hours"
                  inputMode="decimal"
                  className={smallInputClass}
                  value={p.hours}
                  onChange={(e) => setPrograms(programs.map((x, j) => (j === i ? { ...x, hours: e.target.value } : x)))}
                />
              </div>
              <button
                type="button"
                aria-label={`Remove ${p.name || 'program'}`}
                onClick={() => setPrograms(programs.filter((_, j) => j !== i))}
                className="kc-key kc-key--icon"
              >
                ×
              </button>
            </div>
          ))}
          <button
            type="button"
            onClick={() => setPrograms([...programs, { name: '', hours: '' }])}
            className="kc-key kc-key--sm"
          >
            + Add program
          </button>
        </div>
      </Group>

      <Group title="Notifications">
        <p className="kc-caption kc-caption--body">
          {env.dry_run
            ? 'Dry run is on: messages are logged, not sent.'
            : `Discord bot ${env.discord_bot ? 'connected' : 'not set up'} · Slack bot ${env.slack_bot ? 'connected' : 'not set up'}`}
        </p>
        <Field
          label="Discord channel ID"
          hint={env.discord_channel_fallback && <span className="kc-caption kc-caption--body block">Blank uses {env.discord_channel_fallback} from the server config.</span>}
        >
          <input className={smallInputClass} value={s.discord_channel_id} onChange={(e) => setS({ ...s, discord_channel_id: e.target.value })} />
        </Field>
        <Field
          label="Slack channel ID"
          hint={env.slack_channel_fallback && <span className="kc-caption kc-caption--body block">Blank uses {env.slack_channel_fallback} from the server config.</span>}
        >
          <input className={smallInputClass} value={s.slack_channel_id} onChange={(e) => setS({ ...s, slack_channel_id: e.target.value })} />
        </Field>
        <p className="kc-caption kc-caption--body">A new channel is used for firings that haven’t been announced yet.</p>
      </Group>

      <div className="kc-savebar space-y-2">
        {msg && <Alert kind={msg.kind}>{msg.text}</Alert>}
        <button disabled={busy} className="kc-key kc-key--block kc-key--fire">
          {busy ? 'Saving…' : 'Save settings'}
        </button>
      </div>
    </form>
  )
}

function Group({ title, open, children }: { title: string; open?: boolean; children: ReactNode }) {
  return (
    <details className="kc-plate kc-plate--pad" open={open}>
      <summary className="kc-title kc-title--sm cursor-pointer">{title}</summary>
      <div className="mt-4 space-y-3">{children}</div>
    </details>
  )
}

// ── Kilns ───────────────────────────────────────────────────────────────────

const emptyKiln: KilnInput = {
  name: '',
  active: true,
  interior_width_in: null,
  interior_depth_in: null,
  interior_height_in: null,
  shelf_count: null,
  share_per_shelf_pct: null,
}

function KilnsSection({ kilns, onChanged }: { kilns: Kiln[]; onChanged: (k: Kiln[]) => void }) {
  const [editing, setEditing] = useState<number | 'new' | null>(null)
  return (
    <section className="space-y-3">
      <h2 className="kc-title kc-title--sm">Kilns</h2>
      <p className="kc-caption kc-caption--body">
        Size and shelves show on the propose and request forms to help people estimate their share.
      </p>
      {kilns.map((k) =>
        editing === k.id ? (
          <KilnForm
            key={k.id}
            initial={k}
            onCancel={() => setEditing(null)}
            onSave={async (input) => {
              const saved = await updateKiln(k.id, input)
              onChanged(kilns.map((x) => (x.id === k.id ? saved : x)))
              setEditing(null)
            }}
          />
        ) : (
          <button key={k.id} onClick={() => setEditing(k.id)} className="kc-plate kc-plate--pad kc-plate--link w-full space-y-1 text-left">
            <span className="flex items-center justify-between gap-2">
              <span className="kc-label kc-label--lg">{k.name}</span>
              {!k.active && <span className="kc-tag kc-tag--void">Inactive</span>}
            </span>
            <span className="kc-caption kc-caption--body block">{describeKiln(k)}</span>
          </button>
        ),
      )}
      {editing === 'new' ? (
        <KilnForm
          initial={emptyKiln}
          onCancel={() => setEditing(null)}
          onSave={async (input) => {
            const saved = await createKiln(input)
            onChanged([...kilns, saved])
            setEditing(null)
          }}
        />
      ) : (
        <button onClick={() => setEditing('new')} className="kc-key kc-key--block">
          + Add a kiln
        </button>
      )}
    </section>
  )
}

function describeKiln(k: Kiln): string {
  const dims = [k.interior_width_in, k.interior_depth_in, k.interior_height_in]
  const parts: string[] = []
  if (dims.every((d) => d)) parts.push(`${dims.join('" × ')}" (W × D × H)`)
  else if (k.interior_width_in && k.interior_depth_in) parts.push(`${k.interior_width_in}" × ${k.interior_depth_in}"`)
  if (k.shelf_count) parts.push(`${k.shelf_count} shelves`)
  if (k.share_per_shelf_pct) parts.push(`1 shelf ≈ ${k.share_per_shelf_pct}%`)
  return parts.join(' · ') || 'No size set. Tap to add one.'
}

type KilnDraft = Record<Exclude<keyof KilnInput, 'name' | 'active'>, string>

function KilnForm({ initial, onSave, onCancel }: { initial: KilnInput; onSave: (k: KilnInput) => Promise<void>; onCancel: () => void }) {
  const [name, setName] = useState(initial.name)
  const [active, setActive] = useState(initial.active)
  const str = (v: number | null) => (v == null ? '' : String(v))
  const [d, setD] = useState<KilnDraft>({
    interior_width_in: str(initial.interior_width_in),
    interior_depth_in: str(initial.interior_depth_in),
    interior_height_in: str(initial.interior_height_in),
    shelf_count: str(initial.shelf_count),
    share_per_shelf_pct: str(initial.share_per_shelf_pct),
  })
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const n = (v: string) => (v.trim() === '' ? null : Number(v))
  const input = (k: keyof KilnDraft, label: string) => (
    <Field label={label}>
      <input inputMode="decimal" className={smallInputClass} value={d[k]} onChange={(e) => setD({ ...d, [k]: e.target.value })} />
    </Field>
  )

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await onSave({
        name,
        active,
        interior_width_in: n(d.interior_width_in),
        interior_depth_in: n(d.interior_depth_in),
        interior_height_in: n(d.interior_height_in),
        shelf_count: n(d.shelf_count),
        share_per_shelf_pct: n(d.share_per_shelf_pct),
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="kc-plate kc-plate--pad space-y-3">
      <Field label="Name">
        <input required className={smallInputClass} value={name} onChange={(e) => setName(e.target.value)} />
      </Field>
      <div className="grid grid-cols-3 gap-2">
        {input('interior_width_in', 'Width"')}
        {input('interior_depth_in', 'Depth"')}
        {input('interior_height_in', 'Height"')}
      </div>
      <div className="grid grid-cols-2 gap-3">
        {input('shelf_count', 'Shelves')}
        {input('share_per_shelf_pct', '1 shelf ≈ %')}
      </div>
      <label className="kc-check">
        <input type="checkbox" className="kc-checkbox" checked={active} onChange={(e) => setActive(e.target.checked)} />
        <span className="kc-label">Active</span>
        <span className="kc-caption kc-caption--body">(inactive kilns can’t get new firings)</span>
      </label>
      {error && <Alert kind="error">{error}</Alert>}
      <div className="kc-keys kc-keys--fill">
        <button type="button" onClick={onCancel} className="kc-key">
          Cancel
        </button>
        <button disabled={busy} className="kc-key kc-key--fire">
          {busy ? 'Saving…' : 'Save kiln'}
        </button>
      </div>
    </form>
  )
}

// ── Printable QR codes (§9) ─────────────────────────────────────────────────

/** /admin/qr: the wheel QR (/upcoming) and one kiln QR per active kiln, ready to print. */
export function AdminQRPage() {
  const { me } = useSession()
  const { data, error } = useLoad(getSettings, [])
  if (!me?.roles.admin) return <Navigate to="/admin" replace />
  if (error) return <Alert kind="error">{error}</Alert>
  if (!data) return <PageSpinner />
  const base = data.env.base_url.replace(/\/$/, '')
  const active = data.kilns.filter((k) => k.active)
  const isLocal = /localhost|127\.0\.0\.1/.test(base)
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-3 print:hidden">
        <h1 className="kc-title">QR codes</h1>
        <button onClick={() => window.print()} className="kc-key kc-key--sm">
          Print
        </button>
      </div>
      {isLocal && (
        <div className="print:hidden">
          <Alert kind="warning">
            These point at {base}, which phones can’t reach. Set APP_BASE_URL to the real address before printing.
          </Alert>
        </div>
      )}
      <QRCard title="Upcoming firings" subtitle="Scan to see firings and request space" url={`${base}/upcoming`} />
      {active.map((k) => (
        <QRCard
          key={k.id}
          title={active.length > 1 ? `Kiln check-in · ${k.name}` : 'Kiln check-in'}
          subtitle="Scan when loading, starting or unloading"
          url={active.length > 1 ? `${base}/kiln?k=${k.id}` : `${base}/kiln`}
        />
      ))}
    </div>
  )
}

function QRCard({ title, subtitle, url }: { title: string; subtitle: string; url: string }) {
  const svg = renderSVG(url, { border: 2 })
  return (
    <figure className="kc-plate kc-plate--pad break-inside-avoid space-y-2 text-center print:border-0">
      <div className="mx-auto w-64 max-w-full" dangerouslySetInnerHTML={{ __html: svg }} />
      <figcaption>
        <p className="kc-title kc-title--sm">{title}</p>
        <p className="kc-caption kc-caption--body">{subtitle}</p>
        <p className="kc-data break-all">{url}</p>
      </figcaption>
    </figure>
  )
}
