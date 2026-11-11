import { Link, useNavigate } from 'react-router'
import { logout } from '../lib/api'
import { useSession } from '../lib/session'

export default function HomePage() {
  const { me, refresh } = useSession()
  const navigate = useNavigate()
  if (!me) return null

  const roles = [
    'Member',
    me.roles.self_loader && 'Self-loader',
    me.roles.kiln_master && 'Kiln master',
    me.roles.admin && 'Admin',
  ].filter(Boolean) as string[]

  async function signOut() {
    await logout()
    await refresh()
    navigate('/upcoming')
  }

  return (
    <div className="space-y-5">
      <section className="card">
        <p className="text-sm text-stone-500">Signed in as</p>
        <h1 className="text-2xl font-bold">{me.display_name}</h1>
        <p className="text-stone-600">{me.community === 'artlab' ? 'ArtLab · Discord' : 'BMS · Slack'}</p>
        <ul className="mt-3 flex flex-wrap gap-2">
          {roles.map((r) => (
            <li key={r} className="rounded-full bg-stone-900 px-3 py-1 text-sm font-semibold text-white">
              {r}
            </li>
          ))}
        </ul>
      </section>

      <div className="grid gap-3">
        <Link to="/upcoming" className="btn bg-orange-600 text-white">
          Upcoming firings
        </Link>
        <Link to="/kiln" className="btn border-2 border-stone-900 bg-white">
          Kiln check-in
        </Link>
        <button onClick={signOut} className="btn text-stone-600">
          Sign out
        </button>
      </div>
    </div>
  )
}
