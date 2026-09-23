/** How long a new token lasts. These four are the whole set the API accepts. */
export type TokenLifetime = '15d' | '30d' | '1y' | 'never'

export const TOKEN_LIFETIMES: { value: TokenLifetime; label: string; detail: string }[] = [
  { value: '15d', label: '15 days', detail: 'Good for a one-off script or a trial integration.' },
  { value: '30d', label: '30 days', detail: 'A month of access, then it stops working on its own.' },
  { value: '1y', label: '1 year', detail: 'For an integration you expect to keep running.' },
  { value: 'never', label: 'Never expires', detail: 'Only revoking it will stop it. Store it somewhere safe.' },
]

export interface ApiToken {
  id: string
  user_id: string
  /** null means the token reaches every mail account you own. */
  mail_account_id: string | null
  name: string
  /** Masked form, e.g. "xm_7Kd9…Qw2p". The secret is shown once, at creation. */
  hint: string
  /** null means the token never expires. */
  expires_at: string | null
  last_used_at: string | null
  revoked_at: string | null
  created_at: string
  expires_in: TokenLifetime
}

export interface CreatedApiToken {
  api_token: ApiToken
  /** The secret. Returned here and nowhere else, ever again. */
  token: string
}
