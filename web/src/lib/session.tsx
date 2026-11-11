import { createContext, use, useCallback, useEffect, useState, type ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router'
import { getMe } from './api'
import type { Me } from './types'

interface Session {
  me: Me | null
  loading: boolean
  refresh: () => Promise<void>
}

const SessionContext = createContext<Session | null>(null)

export function SessionProvider({ children }: { children: ReactNode }) {
  const [me, setMe] = useState<Me | null>(null)
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(async () => {
    try {
      setMe(await getMe())
    } catch {
      setMe(null)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  return <SessionContext value={{ me, loading, refresh }}>{children}</SessionContext>
}

export function useSession(): Session {
  const s = use(SessionContext)
  if (!s) throw new Error('useSession outside SessionProvider')
  return s
}

/** Renders children only when signed in; otherwise sends the user to /login and back. */
export function RequireLogin({ children }: { children: ReactNode }) {
  const { me, loading } = useSession()
  const loc = useLocation()
  if (loading) return <PageSpinner />
  if (!me) {
    const returnTo = loc.pathname + loc.search
    return <Navigate to={`/login?return_to=${encodeURIComponent(returnTo)}`} replace />
  }
  return children
}

export function PageSpinner() {
  return (
    <div className="flex min-h-[50vh] items-center justify-center" role="status" aria-label="Loading">
      <div className="size-10 animate-spin rounded-full border-4 border-stone-300 border-t-orange-600" />
    </div>
  )
}
