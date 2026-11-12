import { useEffect, useState, type FormEvent } from 'react'
import { Navigate, useNavigate, useSearchParams } from 'react-router'
import { devLogin, getAuthProviders, loginURL } from '../lib/api'
import { PageSpinner, useSession } from '../lib/session'
import type { AuthProviders } from '../lib/types'

// Fixed messages keyed by code; never render free text from the URL.
const ERROR_MESSAGES: Record<string, string> = {
  not_in_guild: 'You need to be a member of the ArtLab Discord server to sign in with Discord.',
  wrong_workspace: 'Please sign in with the BMS Slack workspace.',
  slack_guest: "Slack guest accounts can't sign in. Ask a BMS admin about a full membership.",
  slack_deactivated: 'That Slack account has been deactivated.',
  expired: 'That sign-in link expired. Please try again.',
  cancelled: 'Sign-in was cancelled.',
  failed: 'Something went wrong signing in. Please try again.',
}

/** Only same-origin relative paths; mirrors the server's safeReturnTo. */
function safeReturnTo(raw: string | null): string {
  if (!raw || !raw.startsWith('/') || raw.startsWith('//') || raw.startsWith('/\\')) return '/'
  return raw
}

export default function LoginPage() {
  const [params] = useSearchParams()
  const { me, loading } = useSession()
  const [providers, setProviders] = useState<AuthProviders | null>(null)
  const returnTo = safeReturnTo(params.get('return_to'))
  const errorCode = params.get('error')
  const error = errorCode ? (ERROR_MESSAGES[errorCode] ?? ERROR_MESSAGES.failed) : null

  useEffect(() => {
    getAuthProviders()
      .then(setProviders)
      .catch(() => setProviders({ discord: false, slack: false, dev: false }))
  }, [])

  if (loading || !providers) return <PageSpinner />
  if (me) return <Navigate to={returnTo} replace />

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">Sign in</h1>
        <p className="mt-1 text-stone-600">Use the community you belong to.</p>
      </div>

      {error && (
        <div role="alert" className="rounded-xl border-2 border-red-300 bg-red-50 p-4 font-medium text-red-900">
          {error}
        </div>
      )}

      <div className="space-y-3">
        {providers.discord && (
          <a href={loginURL('discord', returnTo)} className="btn bg-[#5865F2] text-white">
            <DiscordIcon />
            <span>
              ArtLab <span className="font-normal opacity-90">· Discord</span>
            </span>
          </a>
        )}
        {providers.slack && (
          <a href={loginURL('slack', returnTo)} className="btn border-2 border-stone-900 bg-white text-stone-900">
            <SlackIcon />
            <span>
              BMS <span className="font-normal opacity-70">· Slack</span>
            </span>
          </a>
        )}
        {!providers.discord && !providers.slack && !providers.dev && (
          <p className="text-stone-600">No sign-in methods are configured.</p>
        )}
      </div>

      {providers.dev && <DevLoginForm returnTo={returnTo} />}
    </div>
  )
}

function DevLoginForm({ returnTo }: { returnTo: string }) {
  const navigate = useNavigate()
  const { refresh } = useSession()
  const [name, setName] = useState('Cody')
  const [community, setCommunity] = useState<'artlab' | 'bms'>('artlab')
  const [kilnMaster, setKilnMaster] = useState(false)
  const [admin, setAdmin] = useState(false)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setErr(null)
    try {
      await devLogin({ name, community, kiln_master: kilnMaster, admin })
      await refresh()
      navigate(returnTo, { replace: true })
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Dev login failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="card space-y-4 border-2 border-dashed border-amber-400 bg-amber-50">
      <div>
        <h2 className="font-bold text-amber-900">Dev login</h2>
        <p className="text-sm text-amber-800">DEV_LOGIN is on. Sign in as anyone. Never enable in production.</p>
      </div>
      <label className="block">
        <span className="text-sm font-medium">Name</span>
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="mt-1 block w-full rounded-lg border border-stone-300 px-3 py-3 text-lg"
          required
          maxLength={60}
        />
      </label>
      <div className="flex gap-2">
        {(['artlab', 'bms'] as const).map((c) => (
          <button
            key={c}
            type="button"
            onClick={() => {
              setCommunity(c)
              if (c === 'bms') setKilnMaster(false)
            }}
            className={`flex-1 rounded-lg border-2 py-3 font-semibold ${community === c ? 'border-stone-900 bg-stone-900 text-white' : 'border-stone-300 bg-white'}`}
          >
            {c === 'artlab' ? 'ArtLab (Discord)' : 'BMS (Slack)'}
          </button>
        ))}
      </div>
      <div className="space-y-2">
        <Check label="Kiln master" checked={kilnMaster} onChange={setKilnMaster} disabled={community === 'bms'} />
        <Check label="Admin" checked={admin} onChange={setAdmin} />
      </div>
      {err && <p className="text-sm text-red-700">{err}</p>}
      <button disabled={busy} className="btn bg-amber-500 text-stone-900">
        Sign in as {name || '…'}
      </button>
    </form>
  )
}

function Check(props: { label: string; checked: boolean; onChange: (v: boolean) => void; disabled?: boolean }) {
  return (
    <label className={`flex items-center gap-3 text-lg ${props.disabled ? 'opacity-40' : ''}`}>
      <input
        type="checkbox"
        className="size-6 accent-stone-900"
        checked={props.checked}
        disabled={props.disabled}
        onChange={(e) => props.onChange(e.target.checked)}
      />
      {props.label}
    </label>
  )
}

function DiscordIcon() {
  return (
    <svg viewBox="0 0 24 24" className="size-6" fill="currentColor" aria-hidden>
      <path d="M20.3 4.4A19.8 19.8 0 0 0 15.4 3l-.6 1.3a18.4 18.4 0 0 0-5.6 0L8.6 3a19.7 19.7 0 0 0-4.9 1.5C.6 9.1-.3 13.6.1 18.1A19.9 19.9 0 0 0 6.2 21l1.3-2.1c-.7-.3-1.4-.6-2-1l.5-.4a14.2 14.2 0 0 0 12 0l.5.4c-.6.4-1.3.7-2 1L17.8 21a19.8 19.8 0 0 0 6-3c.5-5.2-.9-9.7-3.5-13.6ZM8 15.3c-1.2 0-2.2-1.1-2.2-2.4S6.8 10.5 8 10.5s2.2 1.1 2.2 2.4-1 2.4-2.2 2.4Zm8 0c-1.2 0-2.2-1.1-2.2-2.4s1-2.4 2.2-2.4 2.2 1.1 2.2 2.4-1 2.4-2.2 2.4Z" />
    </svg>
  )
}

function SlackIcon() {
  return (
    <svg viewBox="0 0 24 24" className="size-6" aria-hidden>
      <path fill="#E01E5A" d="M5 15.2a2.1 2.1 0 1 1-2.1-2.1H5v2.1Zm1.1 0a2.1 2.1 0 0 1 4.2 0v5.3a2.1 2.1 0 1 1-4.2 0v-5.3Z" />
      <path fill="#36C5F0" d="M8.2 5a2.1 2.1 0 1 1 2.1-2.1V5H8.2Zm0 1.1a2.1 2.1 0 0 1 0 4.2H2.9a2.1 2.1 0 1 1 0-4.2h5.3Z" />
      <path fill="#2EB67D" d="M18.9 8.2a2.1 2.1 0 1 1 2.1 2.1h-2.1V8.2Zm-1.1 0a2.1 2.1 0 0 1-4.2 0V2.9a2.1 2.1 0 1 1 4.2 0v5.3Z" />
      <path fill="#ECB22E" d="M15.7 18.9a2.1 2.1 0 1 1-2.1 2.1v-2.1h2.1Zm0-1.1a2.1 2.1 0 0 1 0-4.2h5.3a2.1 2.1 0 1 1 0 4.2h-5.3Z" />
    </svg>
  )
}
