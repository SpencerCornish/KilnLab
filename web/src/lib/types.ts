export type Community = 'artlab' | 'bms'

export interface Roles {
  self_loader: boolean
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
