import { useEffect, useState, type FormEvent } from 'react'
import { Navigate, useNavigate, useSearchParams } from 'react-router'
import { Alert } from '../components/firing'
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
        <h1 className="kc-title">Sign in</h1>
        <p className="kc-caption kc-caption--body mt-1">Use the community you belong to.</p>
      </div>

      {error && <Alert kind="error">{error}</Alert>}

      <div className="space-y-2">
        {providers.discord && (
          <a href={loginURL('discord', returnTo)} className="kc-key kc-key--block kc-key--lg kc-key--cobalt">
            <DiscordIcon />
            <span>
              ArtLab <span className="font-normal opacity-90">· Discord</span>
            </span>
          </a>
        )}
        {providers.slack && (
          <a href={loginURL('slack', returnTo)} className="kc-key kc-key--block kc-key--lg">
            <SlackIcon />
            <span>
              BMS <span className="font-normal opacity-70">· Slack</span>
            </span>
          </a>
        )}
        {!providers.discord && !providers.slack && !providers.dev && (
          <p className="kc-caption kc-caption--body">No sign-in methods are configured.</p>
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
    <form onSubmit={submit} className="kc-plate kc-plate--pad kc-plate--dashed space-y-4">
      <div>
        <h2 className="kc-label">Dev login</h2>
        <p className="kc-caption kc-caption--body mt-1">DEV_LOGIN is on. Sign in as anyone. Never enable in production.</p>
      </div>
      <label className="kc-field">
        <span className="kc-field__label">Name</span>
        <input value={name} onChange={(e) => setName(e.target.value)} className="kc-input" required maxLength={60} />
      </label>
      <div className="kc-keys kc-keys--fill">
        {(['artlab', 'bms'] as const).map((c) => (
          <button
            key={c}
            type="button"
            aria-pressed={community === c}
            onClick={() => {
              setCommunity(c)
              if (c === 'bms') setKilnMaster(false)
            }}
            className="kc-key"
          >
            {c === 'artlab' ? 'ArtLab (Discord)' : 'BMS (Slack)'}
          </button>
        ))}
      </div>
      <div>
        <Check label="Kiln master" checked={kilnMaster} onChange={setKilnMaster} disabled={community === 'bms'} />
        <Check label="Admin" checked={admin} onChange={setAdmin} />
      </div>
      {err && <Alert kind="error">{err}</Alert>}
      <button disabled={busy} className="kc-key kc-key--block">
        Sign in as {name || '…'}
      </button>
    </form>
  )
}

function Check(props: { label: string; checked: boolean; onChange: (v: boolean) => void; disabled?: boolean }) {
  return (
    <label className={`kc-check ${props.disabled ? 'opacity-40' : ''}`}>
      <input
        type="checkbox"
        className="kc-checkbox"
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
