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

  const roles = [
    me.roles.kiln_master && 'Kiln master',
    me.roles.admin && 'Admin',
  ].filter(Boolean) as string[]

  async function signOut() {
    await logout()
    await refresh()
    navigate('/upcoming')
  }

  const [firings, options] = data ?? [[], null]
  const mine = firings.filter((f) => f.is_mine && !DONE.has(f.status))
  const upcoming = firings.filter((f) => !f.is_mine && !DONE.has(f.status))
  const recent = firings.filter((f) => DONE.has(f.status))

  return (
    <div className="space-y-6">
      <section className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <h1 className="truncate text-2xl font-bold">Hi, {me.display_name}</h1>
          <p className="text-sm text-stone-600">
            {me.community === 'artlab' ? 'ArtLab · Discord' : 'BMS · Slack'}
            {roles.length > 0 && ` · ${roles.join(', ')}`}
          </p>
        </div>
        <button onClick={signOut} className="shrink-0 rounded-lg px-3 py-2 text-sm font-semibold text-stone-600">
          Sign out
        </button>
      </section>

      <div className="grid grid-cols-2 gap-3">
        <Link to="/firings/new" className="btn col-span-2 bg-orange-600 text-white">
          + Propose a firing
        </Link>
        <Link to="/kiln" className="btn border-2 border-stone-900 bg-white text-base">
          Kiln check-in
        </Link>
        <Link to="/upcoming" className="btn border-2 border-stone-300 bg-white text-base">
          Public page
        </Link>
        {(me.roles.kiln_master || me.roles.admin) && (
          <Link to="/admin" className="btn col-span-2 border-2 border-stone-300 bg-white text-base">
            Admin
          </Link>
        )}
      </div>

      {error && <Alert kind="error">Couldn't load firings: {error}</Alert>}
      {!data && !error && <PageSpinner />}

      {options && (
        <>
          <FiringList title="My firings" empty="You're not in any firings yet." firings={mine} options={options} />
          <FiringList
            title="Upcoming firings"
            empty="Nothing else scheduled. Propose one!"
            firings={upcoming}
            options={options}
          />
          {recent.length > 0 && <FiringList title="Recently finished" firings={recent} options={options} />}
        </>
      )}
    </div>
  )
}

function FiringList({ title, empty, firings, options }: { title: string; empty?: string; firings: FiringSummary[]; options: Options }) {
  return (
    <section className="space-y-3">
      <h2 className="text-lg font-bold">{title}</h2>
      {firings.length === 0 ? (
        <p className="text-stone-500">{empty}</p>
      ) : (
        firings.map((f) => <FiringCard key={f.id} firing={f} options={options} />)
      )}
    </section>
  )
}
