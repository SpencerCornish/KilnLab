import { Link, useNavigate } from 'react-router'
import { Alert, FiringCard } from '../components/firing'
import { getOptions, listFirings, logout } from '../lib/api'
import { PageSpinner, useSession } from '../lib/session'
import type { FiringSummary, Options } from '../lib/types'
import { useLoad } from '../lib/useLoad'

const DONE = new Set(['completed', 'cancelled'])

export default function HomePage() {
  const { me, refresh } = useSession()
  const navigate = useNavigate()
  const { data, error } = useLoad(() => Promise.all([listFirings(), getOptions()]), [])
  if (!me) return null

  async function signOut() {
    await logout()
    await refresh()
    navigate('/upcoming')
  }

  const [firings, options] = data ?? [[], null]
  // The API returns firings soonest first; keep that order in one list. Finished
  // firings come oldest-first, so take the last few and show the newest on top.
  const active = firings.filter((f) => !DONE.has(f.status))
  const recent = firings.filter((f) => DONE.has(f.status)).slice(-3).reverse()

  return (
    <div className="space-y-6">
      <section className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="kc-title break-words">Hi, {me.display_name}</h1>
          <p className="kc-caption kc-caption--body mt-1">{me.community === 'artlab' ? 'ArtLab · Discord' : 'BMS · Slack'}</p>
        </div>
        <button onClick={signOut} className="kc-key kc-key--sm shrink-0">
          Sign out
        </button>
      </section>

      <div className="kc-keys kc-keys--fill">
        <Link to="/firings/new" className="kc-key kc-key--block kc-key--fire">
          + Propose a firing
        </Link>
        {me.roles.admin && (
          <Link to="/admin" className="kc-key">
            Admin
          </Link>
        )}
      </div>

      {error && <Alert kind="error">Couldn't load firings: {error}</Alert>}
      {!data && !error && <PageSpinner />}

      {options && (
        <>
          <FiringList title="Upcoming firings" empty="Nothing scheduled. Propose one!" firings={active} options={options} />
          {recent.length > 0 && <FiringList title="Recently finished" firings={recent} options={options} collapsed />}
        </>
      )}
    </div>
  )
}

function FiringList({
  title,
  empty,
  firings,
  options,
  collapsed,
}: {
  title: string
  empty?: string
  firings: FiringSummary[]
  options: Options
  /** Hide the cards behind a disclosure, e.g. for finished firings. */
  collapsed?: boolean
}) {
  const cards =
    firings.length === 0 ? (
      <p className="kc-caption kc-caption--body">{empty}</p>
    ) : (
      firings.map((f) => <FiringCard key={f.id} firing={f} options={options} />)
    )

  if (collapsed) {
    return (
      <details>
        <summary className="kc-title kc-title--sm cursor-pointer">
          {title} <span className="kc-caption kc-caption--body">({firings.length})</span>
        </summary>
        <div className="mt-4 space-y-4">{cards}</div>
      </details>
    )
  }

  return (
    <section className="space-y-4">
      <h2 className="kc-title kc-title--sm">{title}</h2>
      {cards}
    </section>
  )
}
