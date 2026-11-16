import { useState, type FormEvent, type ReactNode } from 'react'
import { Link, Navigate } from 'react-router'
import { renderSVG } from 'uqr'
import { Alert, Field, smallInputClass } from '../components/firing'
import {
  createBlackout,
  createKiln,
  createProfile,
  deactivateProfile,
  deleteBlackout,
  getSettings,
  listBlackouts,
  updateKiln,
  updateProfile,
  updateSettings,
} from '../lib/api'
import { PageSpinner, useSession } from '../lib/session'
import type { AdminSettings, Blackout, Kiln, KilnInput, Profile, ProfileInput, Settings } from '../lib/types'
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
      <ProfilesSection
        kilns={data.kilns}
        profiles={data.profiles}
        types={data.settings.firing_types}
        cones={data.settings.cones}
        onChanged={(profiles) => setData({ ...data, profiles })}
      />
      <BlackoutsSection kilns={data.kilns} />
      <Link to="/admin/qr" className="kc-key kc-key--block">
        Printable QR codes
      </Link>
    </div>
  )
}

// ── Settings form ───────────────────────────────────────────────────────────

function SettingsForm({ data, onSaved }: { data: AdminSettings; onSaved: (d: AdminSettings) => void }) {
  const [s, setS] = useState<Settings>(data.settings)
  const [types, setTypes] = useState(data.settings.firing_types.join(', '))
  const [cones, setCones] = useState(data.settings.cones.join(', '))
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
        <Field
          label="Escalate again after (min)"
          hint={<span className="kc-caption kc-caption--body block">If the owner still hasn't acted, every kiln master and admin is told after this extra delay.</span>}
        >
          <input
            type="number"
            min={0}
            className={smallInputClass}
            value={s.escalation_grace_minutes}
            onChange={num('escalation_grace_minutes')}
          />
        </Field>
        <p className="kc-caption kc-caption--body">
          Reminders go out once a firing is this late being marked started, or this long past its ready-to-unload time.
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

// ── Firing profiles (per kiln) ──────────────────────────────────────────────

function ProfilesSection({
  kilns,
  profiles,
  types,
  cones,
  onChanged,
}: {
  kilns: Kiln[]
  profiles: Profile[]
  types: string[]
  cones: string[]
  onChanged: (p: Profile[]) => void
}) {
  const [editing, setEditing] = useState<number | 'new' | null>(null)
  const kilnName = (id: number) => kilns.find((k) => k.id === id)?.name ?? `Kiln ${id}`
  const ordered = [...profiles].sort((a, b) => a.kiln_id - b.kiln_id || a.name.localeCompare(b.name))

  return (
    <section className="space-y-3">
      <h2 className="kc-title kc-title--sm">Firing profiles</h2>
      <p className="kc-caption kc-caption--body">
        Presets proposers choose from, per kiln. The name is the firing's name; the two times drive the automatic
        cooling and ready-to-unload phases.
      </p>
      {ordered.length === 0 && <p className="kc-caption kc-caption--body">No profiles yet. Add one so people can propose firings.</p>}
      {ordered.map((p) =>
        editing === p.id ? (
          <ProfileForm
            key={p.id}
            kilns={kilns}
            types={types}
            cones={cones}
            initial={p}
            onCancel={() => setEditing(null)}
            onDeactivate={async () => {
              const saved = await deactivateProfile(p.id)
              onChanged(profiles.map((x) => (x.id === p.id ? saved : x)))
              setEditing(null)
            }}
            onSave={async (input) => {
              const saved = await updateProfile(p.id, input)
              onChanged(profiles.map((x) => (x.id === p.id ? saved : x)))
              setEditing(null)
            }}
          />
        ) : (
          <button key={p.id} onClick={() => setEditing(p.id)} className="kc-plate kc-plate--pad kc-plate--link w-full space-y-1 text-left">
            <span className="flex items-center justify-between gap-2">
              <span className="kc-label kc-label--lg">{p.name}</span>
              <span className="flex items-center gap-2">
                {!p.active && <span className="kc-tag kc-tag--void">Inactive</span>}
                <span className="kc-tag">{kilnName(p.kiln_id)}</span>
              </span>
            </span>
            <span className="kc-caption kc-caption--body block">
              <span className="capitalize">{p.firing_type}</span> · Cone {p.cone} · about{' '}
              {Math.round((p.firing_minutes / 60) * 10) / 10}h firing + {Math.round((p.cooldown_minutes / 60) * 10) / 10}h cooldown
            </span>
          </button>
        ),
      )}
      {editing === 'new' ? (
        <ProfileForm
          kilns={kilns}
          types={types}
          cones={cones}
          onCancel={() => setEditing(null)}
          onSave={async (input) => {
            const saved = await createProfile(input)
            onChanged([...profiles, saved])
            setEditing(null)
          }}
        />
      ) : (
        <button onClick={() => setEditing('new')} className="kc-key kc-key--block">
          + Add a profile
        </button>
      )}
    </section>
  )
}

function ProfileForm({
  kilns,
  types,
  cones,
  initial,
  onSave,
  onCancel,
  onDeactivate,
}: {
  kilns: Kiln[]
  types: string[]
  cones: string[]
  initial?: Profile
  onSave: (p: ProfileInput) => Promise<void>
  onCancel: () => void
  onDeactivate?: () => Promise<void>
}) {
  const [kilnId, setKilnId] = useState(initial?.kiln_id ?? kilns.find((k) => k.active)?.id ?? kilns[0]?.id ?? 0)
  const [name, setName] = useState(initial?.name ?? '')
  const [type, setType] = useState(initial?.firing_type ?? types[0] ?? 'glaze')
  const [cone, setCone] = useState(initial?.cone ?? cones[0] ?? '6')
  const [hours, setHours] = useState(initial ? String(Math.round((initial.firing_minutes / 60) * 100) / 100) : '10')
  const [cooldown, setCooldown] = useState(initial ? String(Math.round((initial.cooldown_minutes / 60) * 100) / 100) : '14')
  const [active, setActive] = useState(initial?.active ?? true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await onSave({
        kiln_id: kilnId,
        name,
        firing_type: type,
        cone,
        firing_minutes: Math.round(Number(hours) * 60),
        cooldown_minutes: Math.round(Number(cooldown) * 60),
        active,
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="kc-plate kc-plate--pad space-y-3">
      <Field label="Kiln">
        <select className={smallInputClass} value={kilnId} onChange={(e) => setKilnId(Number(e.target.value))}>
          {kilns.map((k) => (
            <option key={k.id} value={k.id}>
              {k.name}
            </option>
          ))}
        </select>
      </Field>
      <Field label="Firing name">
        <input required maxLength={120} className={smallInputClass} value={name} onChange={(e) => setName(e.target.value)} placeholder="Cone 06 Bisque — fast" />
      </Field>
      <div className="grid grid-cols-2 items-end gap-3">
        <Field label="Type">
          <select className={`${smallInputClass} capitalize`} value={type} onChange={(e) => setType(e.target.value)}>
            {types.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Cone">
          <select className={smallInputClass} value={cone} onChange={(e) => setCone(e.target.value)}>
            {cones.map((c) => (
              <option key={c} value={c}>
                Cone {c}
              </option>
            ))}
          </select>
        </Field>
      </div>
      <div className="grid grid-cols-2 items-end gap-3">
        <Field label="Firing time (hours)">
          <input required type="number" inputMode="decimal" min={0.01} max={168} step={0.25} className={smallInputClass} value={hours} onChange={(e) => setHours(e.target.value)} />
        </Field>
        <Field label="Cooldown (hours)">
          <input required type="number" inputMode="decimal" min={0} max={168} step={0.25} className={smallInputClass} value={cooldown} onChange={(e) => setCooldown(e.target.value)} />
        </Field>
      </div>
      <label className="kc-check">
        <input type="checkbox" checked={active} onChange={(e) => setActive(e.target.checked)} className="kc-checkbox" />
        Active (offer this profile to proposers)
      </label>
      {error && <Alert kind="error">{error}</Alert>}
      <div className="kc-keys kc-keys--fill">
        <button type="submit" disabled={busy} className="kc-key kc-key--go">
          {busy ? 'Saving…' : 'Save profile'}
        </button>
        <button type="button" onClick={onCancel} className="kc-key">
          Cancel
        </button>
        {onDeactivate && active && (
          <button type="button" disabled={busy} onClick={() => void onDeactivate()} className="kc-key kc-key--danger">
            Deactivate
          </button>
        )}
      </div>
    </form>
  )
}

// ── Blackout dates ──────────────────────────────────────────────────────────

/** Admin-only blocks: days a kiln can't take new firings (maintenance, etc.). */
function BlackoutsSection({ kilns }: { kilns: Kiln[] }) {
  const { data, error, setData } = useLoad(listBlackouts, [])
  const [kilnId, setKilnId] = useState(kilns.find((k) => k.active)?.id ?? kilns[0]?.id ?? 0)
  const [start, setStart] = useState('')
  const [end, setEnd] = useState('')
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState<{ kind: 'error' | 'info'; text: string } | null>(null)
  const kilnName = (id: number) => kilns.find((k) => k.id === id)?.name ?? `Kiln ${id}`

  async function add(e: FormEvent) {
    e.preventDefault()
    if (!start || !end) return setMsg({ kind: 'error', text: 'Pick a first and last day.' })
    setBusy(true)
    setMsg(null)
    try {
      const created = await createBlackout({ kiln_id: kilnId, start_date: start, end_date: end, reason: reason.trim() })
      setData((prev) =>
        [...(prev ?? []), created].sort((a, b) => a.kiln_id - b.kiln_id || a.start_date.localeCompare(b.start_date)),
      )
      setStart('')
      setEnd('')
      setReason('')
      setMsg({ kind: 'info', text: 'Blackout added.' })
    } catch (err) {
      setMsg({ kind: 'error', text: err instanceof Error ? err.message : String(err) })
    } finally {
      setBusy(false)
    }
  }

  async function remove(b: Blackout) {
    setMsg(null)
    try {
      await deleteBlackout(b.id)
      setData((prev) => (prev ?? []).filter((x) => x.id !== b.id))
    } catch (err) {
      setMsg({ kind: 'error', text: err instanceof Error ? err.message : String(err) })
    }
  }

  return (
    <section className="space-y-3">
      <h2 className="kc-title kc-title--sm">Blackout dates</h2>
      <p className="kc-caption kc-caption--body">
        Days a kiln can't take new firings (maintenance and other needs). Blacked-out days can't be proposed or
        rescheduled onto; firings already booked are left alone.
      </p>
      {error && <Alert kind="error">{error}</Alert>}
      {data && data.length > 0 ? (
        <ul className="space-y-2">
          {data.map((b) => (
            <li key={b.id} className="kc-plate kc-plate--pad flex items-center justify-between gap-2">
              <span className="min-w-0">
                <span className="kc-label">{b.start_date === b.end_date ? b.start_date : `${b.start_date} → ${b.end_date}`}</span>
                <span className="kc-caption kc-caption--body block">
                  {kilnName(b.kiln_id)}
                  {b.reason ? ` · ${b.reason}` : ''}
                </span>
              </span>
              <button type="button" onClick={() => remove(b)} className="kc-key kc-key--sm shrink-0">
                Remove
              </button>
            </li>
          ))}
        </ul>
      ) : (
        <p className="kc-caption kc-caption--body">No blackout dates.</p>
      )}
      <form onSubmit={add} className="kc-plate kc-plate--pad space-y-3">
        {kilns.length > 1 && (
          <Field label="Kiln">
            <select className={smallInputClass} value={kilnId} onChange={(e) => setKilnId(Number(e.target.value))}>
              {kilns.map((k) => (
                <option key={k.id} value={k.id}>
                  {k.name}
                </option>
              ))}
            </select>
          </Field>
        )}
        <div className="grid grid-cols-2 items-end gap-3">
          <Field label="First day">
            <input type="date" required className={smallInputClass} value={start} onChange={(e) => setStart(e.target.value)} />
          </Field>
          <Field label="Last day">
            <input type="date" required className={smallInputClass} value={end} onChange={(e) => setEnd(e.target.value)} />
          </Field>
        </div>
        <Field label="Reason (optional)">
          <input
            maxLength={500}
            className={smallInputClass}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="Element replacement"
          />
        </Field>
        {msg && <Alert kind={msg.kind}>{msg.text}</Alert>}
        <button disabled={busy} className="kc-key kc-key--block">
          {busy ? 'Adding…' : '+ Add blackout'}
        </button>
      </form>
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
