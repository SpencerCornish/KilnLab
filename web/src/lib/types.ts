export type Community = 'artlab' | 'bms'

export interface Roles {
  kiln_master: boolean
  admin: boolean
}

export interface Me {
  id: number
  display_name: string
  avatar_url: string | null
  community: Community
  roles: Roles
}

export interface AuthProviders {
  discord: boolean
  slack: boolean
  dev: boolean
}

// ── Firings (mirrors internal/api/firings.go) ───────────────────────────────

export type FiringStatus =
  | 'proposed'
  | 'on_hold'
  | 'confirmed'
  | 'loading'
  | 'loaded'
  | 'firing'
  | 'cooling'
  | 'completed'
  | 'cancelled'

export type RequestStatus = 'pending' | 'approved' | 'declined' | 'withdrawn'

export interface UserRef {
  id: number
  name: string
}

export interface Fullness {
  calculated_pct: number
  display_pct: number
  overridden: boolean
  room_left_pct: number
}

export interface Firing {
  id: number
  kiln_id: number
  title: string | null
  firing_type: string
  cone: string
  notes: string
  /** YYYY-MM-DD: the day it's booked for. No start time is promised. */
  scheduled_date: string
  status: FiringStatus
  hold_reason: string | null
  cancel_reason: string | null
  proposer: UserRef
  proposer_description: string
  proposer_share_pct: number
  proposer_cone: string
  proposer_clay_body: string | null
  fullness: Fullness
  requests_close_at: string
  requests_closed_manually: boolean
  requests_open: boolean
  program: string | null
  estimated_duration_minutes: number | null
  started_at: string | null
  estimated_unload_at: string | null
  cooling_at: string | null
  unloaded_at: string | null
  ready_for_pickup_at: string | null
  completed_at: string | null
  created_at: string
}

export type TimestampField = 'started_at' | 'cooling_at' | 'unloaded_at'

export interface Participant {
  id: number
  name: string
  share_pct: number
}

export interface FiringSummary extends Firing {
  approved_count: number
  pending_count: number
  is_mine: boolean
  /** Everyone with work in the firing, biggest share first. */
  participants: Participant[]
}

export interface FiringRequest {
  id: number
  user: UserRef
  user_avatar_url: string | null
  user_community: Community
  description: string
  estimated_share_pct: number
  cone: string
  clay_body: string | null
  glazed: boolean
  status: RequestStatus
  decision_note?: string | null
  decided_at: string | null
  compatibility_warning?: string | null
  created_at: string
  can: { edit: boolean; withdraw: boolean; decide: boolean }
}

export interface FiringEvent {
  id: number
  type: string
  actor: UserRef | null
  from_status: FiringStatus | null
  to_status: FiringStatus | null
  occurred_at: string
  note: string | null
  source: 'web' | 'kiln_qr' | 'system'
}

export interface FiringPerms {
  request: boolean
  manage: boolean
  confirm: boolean
  hold: boolean
  release: boolean
  reschedule: boolean
  cancel: boolean
  next_steps: FiringStatus[]
  blocked_reason: string
  backdate: boolean
  correct_timestamps: boolean
}

export interface FiringDetail {
  firing: Firing
  requests: FiringRequest[]
  my_request: FiringRequest | null
  events: FiringEvent[]
  can: FiringPerms
}

export interface MutationResult extends FiringDetail {
  warnings: string[]
}

export interface Kiln {
  id: number
  name: string
  active: boolean
  interior_width_in: number | null
  interior_depth_in: number | null
  interior_height_in: number | null
  shelf_count: number | null
  share_per_shelf_pct: number | null
}

export interface Options {
  timezone: string
  min_notice_hours: number
  request_cutoff_hours: number
  firing_types: string[]
  cones: string[]
  /** Active kilns first; inactive ones are kept so old firings still show their kiln. */
  kilns: Kiln[]
  programs: Program[]
}

export interface Program {
  name: string
  default_duration_minutes?: number
}

export interface KilnFiring {
  id: number
  kiln_id: number
  title: string | null
  firing_type: string
  cone: string
  /** YYYY-MM-DD: the day it's booked for. No start time is promised. */
  scheduled_date: string
  status: FiringStatus
  proposer_name: string
}

export interface KilnView {
  kiln: Kiln | null
  firings: KilnFiring[]
}

export interface AdvanceInput {
  to: FiringStatus
  occurred_at?: string | null
  program?: string | null
  estimated_duration_minutes?: number | null
  note?: string | null
  source: 'web' | 'kiln_qr'
}

export interface AdminUser {
  id: number
  display_name: string
  avatar_url: string | null
  community: Community
  provider: 'discord' | 'slack'
  is_kiln_master: boolean
  is_admin: boolean
  last_login_at: string
}

export interface NewFiring {
  kiln_id: number
  firing_type: string
  cone: string
  title: string | null
  notes: string
  /** YYYY-MM-DD: the day it's booked for. No start time is promised. */
  scheduled_date: string
  description: string
  share_pct: number
  proposer_cone: string
  clay_body: string | null
}

export interface RequestInput {
  description: string
  share_pct: number
  cone: string
  clay_body: string | null
  glazed: boolean
}

export interface OutboxItem {
  id: number
  kind: 'channel_upsert' | 'dm'
  platform: 'discord' | 'slack'
  status: 'pending' | 'sent' | 'failed'
  attempts: number
  last_error: string | null
  next_attempt_at: string
  created_at: string
  payload: { firing_id?: number; user_id?: number; thread?: string; text?: string }
}

export interface NotificationLog {
  dry_run: boolean
  discord: boolean
  slack: boolean
  items: OutboxItem[]
}

// ── Public page (§9A) ───────────────────────────────────────────────────────

export interface PublicFiring {
  id: number
  kiln_name?: string
  title: string | null
  firing_type: string
  cone: string
  /** YYYY-MM-DD: the day it's booked for. No start time is promised. */
  scheduled_date: string
  status: FiringStatus
  fullness_pct: number
  room_left_pct: number
  requests_close_at: string
  requests_open: boolean
  participant_count: number
  started_at: string | null
  estimated_unload_at: string | null
}

export interface Upcoming {
  timezone: string
  days_ahead: number
  firings: PublicFiring[]
}

// ── Admin settings ──────────────────────────────────────────────────────────

export interface Settings {
  timezone: string
  min_notice_hours: number
  request_cutoff_hours: number
  underfilled_threshold_pct: number
  firing_types: string[]
  cones: string[]
  programs: Program[]
  discord_channel_id: string
  slack_channel_id: string
  missed_start_grace_minutes: number
  missed_unload_grace_minutes: number
  day_reminder_hour: number
  public_page_days_ahead: number
}

export interface AdminSettings {
  settings: Settings
  kilns: Kiln[]
  env: {
    base_url: string
    discord_bot: boolean
    slack_bot: boolean
    discord_channel_fallback: string
    slack_channel_fallback: string
    dry_run: boolean
  }
}

export type KilnInput = Omit<Kiln, 'id'>
