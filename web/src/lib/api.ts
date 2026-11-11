import type { AuthProviders, Me } from './types'

function csrfToken(): string | undefined {
  const m = document.cookie.match(/(?:^|;\s*)kilnlab_csrf=([^;]+)/)
  return m?.[1] ? decodeURIComponent(m[1]) : undefined
}

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const method = (init?.method ?? 'GET').toUpperCase()
  const headers = new Headers(init?.headers)
  if (init?.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  if (method !== 'GET' && method !== 'HEAD') {
    // The CSRF cookie is set on the first /api response; make sure we have one.
    if (!csrfToken()) await fetch('/api/health', { credentials: 'include' })
    const t = csrfToken()
    if (t) headers.set('X-CSRF-Token', t)
  }
  const res = await fetch(`/api${path}`, { ...init, method, headers, credentials: 'include' })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new ApiError(res.status, body.error ?? res.statusText)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

/** Returns the signed-in user, or null when signed out. */
export async function getMe(): Promise<Me | null> {
  try {
    return await request<Me>('/me')
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) return null
    throw e
  }
}

export const getAuthProviders = () => request<AuthProviders>('/auth/providers')
export const logout = () => request<void>('/auth/logout', { method: 'POST' })

export const devLogin = (body: {
  name: string
  community: 'artlab' | 'bms'
  kiln_master: boolean
  self_loader: boolean
  admin: boolean
}) => request<void>('/auth/dev', { method: 'POST', body: JSON.stringify(body) })

/** Full-page navigation URL that starts OAuth and returns to `returnTo`. */
export function loginURL(provider: 'discord' | 'slack', returnTo: string): string {
  return `/api/auth/${provider}/start?return_to=${encodeURIComponent(returnTo)}`
}
