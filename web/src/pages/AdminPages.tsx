import { useState } from 'react'
import { Link, Navigate } from 'react-router'
import { Alert } from '../components/firing'
import { getNotificationLog, listUsers, signOutUser, updateUser } from '../lib/api'
import { PageSpinner, useSession } from '../lib/session'
import { relative } from '../lib/time'
import type { AdminUser, OutboxItem } from '../lib/types'
import { useLoad } from '../lib/useLoad'

export function AdminIndexPage() {
  const { me } = useSession()
  if (!me?.roles.admin) return <Navigate to="/" replace />
  return (
    <div className="space-y-4">
      <h1 className="kc-title">Admin</h1>
      <Link to="/admin/users" className="kc-key kc-key--block">
        People
      </Link>
      <Link to="/admin/settings" className="kc-key kc-key--block">
        Studio settings &amp; kilns
      </Link>
      <Link to="/admin/qr" className="kc-key kc-key--block">
        Printable QR codes
      </Link>
      <NotificationLogSection />
    </div>
  )
}

const OUTBOX_STATUS: Record<OutboxItem['status'], string> = {
  pending: 'kc-tag--flag',
  sent: 'kc-tag--go',
  failed: 'kc-tag--stop',
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
      <div className="flex items-center justify-between gap-3">
        <h2 className="kc-title kc-title--sm">Notifications</h2>
        <button onClick={() => void reload()} className="kc-key kc-key--sm">
          Refresh
        </button>
      </div>
      <p className="kc-caption kc-caption--body">{mode}</p>
      {data.items.length === 0 && <p className="kc-caption kc-caption--body">Nothing sent yet.</p>}
      <ul className="space-y-2">
        {data.items.map((o) => (
          <li key={o.id} className="kc-plate kc-plate--pad space-y-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className={`kc-tag ${OUTBOX_STATUS[o.status]}`}>{o.status}</span>
              <span className="kc-label capitalize">{o.platform}</span>
              <span className="kc-data kc-muted">
                {o.kind === 'dm' ? 'DM' : 'channel message'}
                {o.payload.firing_id ? ` · firing #${o.payload.firing_id}` : ''}
              </span>
              <span className="kc-data kc-muted ml-auto">{relative(o.created_at)}</span>
            </div>
            {(o.payload.text || o.payload.thread) && (
              <p className="line-clamp-2 kc-caption kc-caption--body">{o.payload.text ?? `Thread: ${o.payload.thread}`}</p>
            )}
            {o.last_error && (
              <p className="kc-warning">
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

  async function signOut(u: AdminUser) {
    if (!window.confirm(`Sign ${u.display_name} out on all their devices?`)) return
    setBusy(u.id)
    setErr(null)
    try {
      await signOutUser(u.id)
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
      <Link to="/admin" className="kc-back">
        ← Admin
      </Link>
      <h1 className="kc-title">People</h1>
      <p className="kc-caption kc-caption--body">
        Admins can manage studio settings and other admins. Kiln masters come from the Kiln Master role in Discord.
        People who leave the Discord server or Slack workspace are signed out automatically within about an hour.
      </p>
      <input
        type="search"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder="Search names"
        className="kc-input"
      />
      {err && <Alert kind="error">{err}</Alert>}
      <ul className="space-y-3">
        {users.map((u) => (
          <li key={u.id} className="kc-plate kc-plate--pad space-y-3">
            <div className="flex items-center gap-3">
              {u.avatar_url && <img src={u.avatar_url} alt="" className="kc-avatar size-10" />}
              <div className="min-w-0">
                <p className="kc-label kc-label--lg truncate">{u.display_name}</p>
                <p className="kc-caption kc-caption--body">
                  {u.community === 'artlab' ? 'ArtLab · Discord' : 'BMS · Slack'} · last seen {relative(u.last_login_at)}
                  {u.is_kiln_master && ' · Kiln master'}
                </p>
              </div>
            </div>
            <div className="kc-keys">
              <Toggle
                label="Admin"
                on={u.is_admin}
                disabled={busy === u.id || u.id === me.id}
                onChange={(v) => toggle(u, { admin: v })}
              />
              <button
                type="button"
                disabled={busy === u.id}
                onClick={() => signOut(u)}
                className="kc-key kc-key--sm"
              >
                Sign out
              </button>
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
      className={`kc-key kc-key--sm${on ? ' kc-key--selected' : ''}`}
    >
      {on ? '✓ ' : ''}
      {label}
    </button>
  )
}
