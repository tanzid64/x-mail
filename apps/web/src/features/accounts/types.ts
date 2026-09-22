export type AccountStatus = 'active' | 'invalid_credentials' | 'disabled'
export type DomainStatus = 'pending' | 'verified' | 'failed'

export interface MailAccount {
  id: string
  name: string
  provider: 'resend'
  provider_account_id: string | null
  /** Masked form of the API key, e.g. "re_123…wxyz". The key itself never leaves the server. */
  api_key_hint: string
  /** Masked signing secret for this account's provider callbacks. */
  webhook_secret_hint: string
  status: AccountStatus
  status_message: string
  last_checked_at: string | null
  color: string
  is_default: boolean
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface DNSRecord {
  type: string
  name: string
  value: string
  ttl?: string
  priority?: number
  status?: string
}

export interface Domain {
  id: string
  mail_account_id: string
  domain: string
  provider_domain_id: string
  status: DomainStatus
  region: string
  records: DNSRecord[]
  last_synced_at: string | null
  created_at: string
  updated_at: string
}

export interface MailAddress {
  id: string
  domain_id: string
  email: string
  display_name: string
  is_default: boolean
  is_active: boolean
  created_at: string
  updated_at: string
  /** Denormalised by the API so the compose picker can group without extra requests. */
  domain: string
  mail_account_id: string
}

/** Palette offered when naming an account, matching the switcher dots. */
export const ACCOUNT_COLORS = [
  'slate',
  'emerald',
  'blue',
  'violet',
  'amber',
  'rose',
] as const

export type AccountColor = (typeof ACCOUNT_COLORS)[number]

/**
 * Tailwind cannot see class names built at runtime, so the colour classes are
 * written out here rather than interpolated.
 */
export const ACCOUNT_DOT_CLASS: Record<string, string> = {
  slate: 'bg-slate-500',
  emerald: 'bg-emerald-500',
  blue: 'bg-blue-500',
  violet: 'bg-violet-500',
  amber: 'bg-amber-500',
  rose: 'bg-rose-500',
}

export function accountDotClass(color: string): string {
  return ACCOUNT_DOT_CLASS[color] ?? ACCOUNT_DOT_CLASS.slate
}
