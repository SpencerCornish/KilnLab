import { useState } from 'react'
import { Link, Navigate } from 'react-router'
import { Alert } from '../components/firing'
import { getNotificationLog, listUsers, updateUser } from '../lib/api'
import { PageSpinner, useSession } from '../lib/session'
import { relative } from '../lib/time'
import type { AdminUser, OutboxItem } from '../lib/types'
import { useLoad } from '../lib/useLoad'

export function AdminIndexPage() {
  const { me } = useSession()
  if (!me || !(me.roles.kiln_master || me.roles.admin)) return <Navigate to="/" replace />
  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-bold">Admin</h1>
      {me.roles.admin && (
        <>
          <Link to="/admin/users" className="btn bg-stone-900 text-white">
            People
          </Link>
          <Link to="/admin/settings" className="btn bg-stone-900 text-white">
            Studio settings &amp; kilns
          </Link>
          <Link to="/admin/qr" className="btn border-2 border-stone-900 bg-white">
            Printable QR codes
          </Link>
          <NotificationLogSection />
        </>
      )}
    </div>
  )
}

const OUTBOX_STATUS: Record<OutboxItem['status'], string> = {
  pending: 'bg-amber-100 text-amber-900',
  sent: 'bg-green-100 text-green-900',
  failed: 'bg-red-100 text-red-900',
}

function NotificationLogSection() {
  const { data, error, reload } = useLoad(getNotificationLog, [])
  if (error) return <Alert kind="error">{error}</Alert>
  if (!data) return <PageSpinner />
  const mode = data.dry_run
    ? 'Dry run: messages are logged, not sent.'
    : [data.discord ? 'Discord on' : 'Discord off (no bot token)', data.slack ? 'Slack on' : 'Slack off (no bot token)'].join(' · ')
  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-bold">Notifications</h2>
        <button onClick={() => void reload()} className="rounded-lg px-3 py-2 text-sm font-semibold text-orange-700">
          Refresh
        </button>
      </div>
      <p className="text-sm text-stone-600">{mode}</p>
      {data.items.length === 0 && <p className="text-stone-500">Nothing sent yet.</p>}
      <ul className="space-y-2">
        {data.items.map((o) => (
          <li key={o.id} className="card space-y-1 py-3">
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <span className={`rounded-full px-2 py-0.5 font-semibold ${OUTBOX_STATUS[o.status]}`}>{o.status}</span>
              <span className="font-semibold capitalize">{o.platform}</span>
              <span className="text-stone-600">
                {o.kind === 'dm' ? 'DM' : 'channel message'}
                {o.payload.firing_id ? ` · firing #${o.payload.firing_id}` : ''}
              </span>
              <span className="ml-auto text-stone-500">{relative(o.created_at)}</span>
            </div>
            {(o.payload.text || o.payload.thread) && (
              <p className="line-clamp-2 text-sm text-stone-700">{o.payload.text ?? `Thread: ${o.payload.thread}`}</p>
            )}
            {o.last_error && (
              <p className="text-sm text-red-800">
                {o.attempts} attempt{o.attempts === 1 ? '' : 's'}: {o.last_error}
              </p>
            )}
          </li>
        ))}
      </ul>
    </section>
  )
}

/** /admin/users: admins toggle admin access. */
export function AdminUsersPage() {
  const { me, refresh } = useSession()
  const { data, error, setData } = useLoad(listUsers, [])
  const [busy, setBusy] = useState<number | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [query, setQuery] = useState('')

  if (!me || !me.roles.admin) return <Navigate to="/" replace />
  if (error) return <Alert kind="error">{error}</Alert>
  if (!data) return <PageSpinner />

  async function toggle(u: AdminUser, patch: { admin: boolean }) {
    setBusy(u.id)
    setErr(null)
    try {
      await updateUser(u.id, patch)
      setData(data!.map((x) => (x.id === u.id ? { ...x, is_admin: patch.admin } : x)))
      if (u.id === me!.id) await refresh()
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(null)
    }
  }

  const q = query.trim().toLowerCase()
  const users = q ? data.filter((u) => u.display_name.toLowerCase().includes(q)) : data

  return (
    <div className="space-y-4">
      <Link to="/admin" className="text-sm font-semibold text-stone-500">
        ← Admin
      </Link>
      <h1 className="text-2xl font-bold">People</h1>
      <p className="text-stone-600">Admins can manage studio settings and other admins. Kiln masters come from the Kiln Master role in Discord.</p>
      <input
        type="search"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder="Search names"
        className="block w-full rounded-xl border-2 border-stone-300 bg-white px-3 py-3 text-lg"
      />
      {err && <Alert kind="error">{err}</Alert>}
      <ul className="space-y-3">
        {users.map((u) => (
          <li key={u.id} className="card space-y-3">
            <div className="flex items-center gap-3">
              {u.avatar_url && <img src={u.avatar_url} alt="" className="size-10 rounded-full" />}
              <div className="min-w-0">
                <p className="truncate text-lg font-semibold">{u.display_name}</p>
                <p className="text-sm text-stone-500">
                  {u.community === 'artlab' ? 'ArtLab · Discord' : 'BMS · Slack'} · last seen {relative(u.last_login_at)}
                  {u.is_kiln_master && ' · Kiln master'}
                </p>
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              <Toggle
                label="Admin"
                on={u.is_admin}
                disabled={busy === u.id || u.id === me.id}
                onChange={(v) => toggle(u, { admin: v })}
              />
            </div>
          </li>
        ))}
      </ul>
    </div>
  )
}

function Toggle({ label, on, disabled, onChange }: { label: string; on: boolean; disabled?: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      disabled={disabled}
      onClick={() => onChange(!on)}
      className={`min-h-12 rounded-full border-2 px-4 font-semibold disabled:opacity-50 ${on ? 'border-green-700 bg-green-700 text-white' : 'border-stone-300 bg-white text-stone-700'}`}
    >
      {on ? '✓ ' : ''}
      {label}
    </button>
  )
}
