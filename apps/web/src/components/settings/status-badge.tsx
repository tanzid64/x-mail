import { Badge } from '@/components/ui/badge'
import type { AccountStatus, DomainStatus } from '@/features/accounts/types'

const ACCOUNT_LABELS: Record<AccountStatus, { label: string; variant: 'default' | 'secondary' | 'destructive' }> = {
  active: { label: 'Connected', variant: 'default' },
  invalid_credentials: { label: 'Key rejected', variant: 'destructive' },
  disabled: { label: 'Disabled', variant: 'secondary' },
}

export function AccountStatusBadge({ status }: { status: AccountStatus }) {
  const { label, variant } = ACCOUNT_LABELS[status] ?? ACCOUNT_LABELS.disabled
  return <Badge variant={variant}>{label}</Badge>
}

const DOMAIN_LABELS: Record<DomainStatus, { label: string; variant: 'default' | 'secondary' | 'destructive' }> = {
  verified: { label: 'Verified', variant: 'default' },
  pending: { label: 'Pending DNS', variant: 'secondary' },
  failed: { label: 'Failed', variant: 'destructive' },
}

export function DomainStatusBadge({ status }: { status: DomainStatus }) {
  const { label, variant } = DOMAIN_LABELS[status] ?? DOMAIN_LABELS.pending
  return <Badge variant={variant}>{label}</Badge>
}
