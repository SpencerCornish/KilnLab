import { Link, Outlet } from 'react-router'
import { useSession } from '../lib/session'

export default function Layout() {
  const { me } = useSession()
  return (
    <div className="mx-auto flex min-h-dvh max-w-xl flex-col">
      <header className="sticky top-0 z-10 flex items-center justify-between bg-stone-900 px-4 py-3 text-white pt-[max(0.75rem,env(safe-area-inset-top))]">
        <Link to="/" className="flex items-center gap-2 text-lg font-bold">
          <img src="/favicon.svg" alt="" className="size-7" />
          KilnLab
        </Link>
        {me ? (
          <Link to="/" className="flex items-center gap-2 text-sm text-stone-300">
            {me.avatar_url && <img src={me.avatar_url} alt="" className="size-8 rounded-full" />}
            <span className="max-w-[10rem] truncate">{me.display_name}</span>
          </Link>
        ) : (
          <Link to="/login" className="rounded-lg px-3 py-2 text-sm font-semibold text-orange-300">
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
