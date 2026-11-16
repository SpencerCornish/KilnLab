import type {
  AdminSettings,
  AdminUser,
  Blackout,
  BlackoutInput,
  Kiln,
  KilnInput,
  Settings,
  Upcoming,
  AdvanceInput,
  AuthProviders,
  KilnView,
  NotificationLog,
  TimestampField,
  FiringDetail,
  FiringSummary,
  Me,
  MutationResult,
  NewFiring,
  Options,
  RequestInput,
} from './types'

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
  admin: boolean
}) => request<void>('/auth/dev', { method: 'POST', body: JSON.stringify(body) })

/** Full-page navigation URL that starts OAuth and returns to `returnTo`. */
export function loginURL(provider: 'discord' | 'slack', returnTo: string): string {
  return `/api/auth/${provider}/start?return_to=${encodeURIComponent(returnTo)}`
}

// ── Firings ─────────────────────────────────────────────────────────────────

let optionsPromise: Promise<Options> | null = null
/** Form options (cones, types, kiln size, timezone). Fetched once per page load. */
export function getOptions(): Promise<Options> {
  optionsPromise ??= request<Options>('/options').catch((e) => {
    optionsPromise = null
    throw e
  })
  return optionsPromise
}

const json = (method: string, body?: unknown): RequestInit => ({
  method,
  body: body === undefined ? undefined : JSON.stringify(body),
})

export const listFirings = () => request<FiringSummary[]>('/firings')
export const getFiring = (id: number) => request<FiringDetail>(`/firings/${id}`)
export const createFiring = (f: NewFiring) => request<FiringDetail>('/firings', json('POST', f))

const f = (id: number) => `/firings/${id}`
export const firingActions = {
  createRequest: (id: number, r: RequestInput) => request<MutationResult>(`${f(id)}/requests`, json('POST', r)),
  updateRequest: (id: number, rid: number, r: RequestInput) =>
    request<MutationResult>(`${f(id)}/requests/${rid}`, json('PUT', r)),
  withdrawRequest: (id: number, rid: number) => request<MutationResult>(`${f(id)}/requests/${rid}/withdraw`, json('POST')),
  decide: (id: number, rid: number, decision: 'approve' | 'decline', note: string | null) =>
    request<MutationResult>(`${f(id)}/requests/${rid}/decide`, json('POST', { decision, note })),
  confirm: (id: number) => request<MutationResult>(`${f(id)}/confirm`, json('POST')),
  hold: (id: number, reason: string) => request<MutationResult>(`${f(id)}/hold`, json('POST', { reason })),
  release: (id: number, scheduledDate: string | null) =>
    request<MutationResult>(`${f(id)}/release`, json('POST', { scheduled_date: scheduledDate })),
  reschedule: (id: number, scheduledDate: string, note: string | null) =>
    request<MutationResult>(`${f(id)}/reschedule`, json('POST', { scheduled_date: scheduledDate, note })),
  cancel: (id: number, reason: string) => request<MutationResult>(`${f(id)}/cancel`, json('POST', { reason })),
  setFullness: (id: number, pct: number | null) =>
    request<MutationResult>(`${f(id)}/fullness-override`, json('PUT', { override_pct: pct })),
  setRequestsClosed: (id: number, closed: boolean) =>
    request<MutationResult>(`${f(id)}/requests-window`, json('PUT', { closed })),
  advance: (id: number, input: AdvanceInput) => request<MutationResult>(`${f(id)}/status`, json('POST', input)),
  correctTimestamp: (id: number, field: TimestampField, at: string) =>
    request<MutationResult>(`${f(id)}/timestamps`, json('PUT', { field, at })),
}

export const getKilnView = (kilnId: string | null) =>
  request<KilnView>(kilnId ? `/kiln?k=${encodeURIComponent(kilnId)}` : '/kiln')

export const listUsers = () => request<AdminUser[]>('/admin/users')
export const updateUser = (id: number, patch: { admin: boolean }) =>
  request<void>(`/admin/users/${id}`, json('PATCH', patch))

export const signOutUser = (id: number) => request<void>(`/admin/users/${id}/sign-out`, { method: 'POST' })

export const getNotificationLog = () => request<NotificationLog>('/admin/notifications')

// ── Public (no login) ───────────────────────────────────────────────────────

export const getUpcoming = () => request<Upcoming>('/public/upcoming')

// ── Admin settings ──────────────────────────────────────────────────────────

/** Settings changes invalidate the cached form options. */
function freshOptions<T>(p: Promise<T>): Promise<T> {
  return p.then((v) => {
    optionsPromise = null
    return v
  })
}

export const getSettings = () => request<AdminSettings>('/admin/settings')
export const updateSettings = (s: Settings) => freshOptions(request<AdminSettings>('/admin/settings', json('PUT', s)))
export const createKiln = (k: KilnInput) => freshOptions(request<Kiln>('/admin/kilns', json('POST', k)))
export const updateKiln = (id: number, k: KilnInput) => freshOptions(request<Kiln>(`/admin/kilns/${id}`, json('PUT', k)))

export const listBlackouts = () => request<Blackout[]>('/admin/blackouts')
export const createBlackout = (b: BlackoutInput) => freshOptions(request<Blackout>('/admin/blackouts', json('POST', b)))
export const deleteBlackout = (id: number) =>
  freshOptions(request<void>(`/admin/blackouts/${id}`, { method: 'DELETE' }))
