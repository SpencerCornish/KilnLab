import { Link, Outlet } from 'react-router'
import { useSession } from '../lib/session'

export default function Layout() {
  const { me } = useSession()
  return (
    <div className="mx-auto flex min-h-dvh max-w-xl flex-col">
      <header className="kc-header">
        <Link to="/" className="kc-header__brand">
          <img src="/favicon.svg" alt="" className="size-7" />
          KilnLab
        </Link>
        {me ? (
          <Link to="/" className="kc-header__user">
            {me.avatar_url && <img src={me.avatar_url} alt="" className="kc-avatar size-8" />}
            <span className="kc-header__name">{me.display_name}</span>
          </Link>
        ) : (
          <Link to="/login" className="kc-key kc-key--sm kc-key--fire">
            Sign in
          </Link>
        )}
      </header>
      <main className="flex-1 px-4 py-5">
        <Outlet />
      </main>
    </div>
  )
}
