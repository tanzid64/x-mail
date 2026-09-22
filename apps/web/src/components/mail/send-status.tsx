import { CircleAlert, Clock, Check, CheckCheck } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import type { Message } from '@/features/mailbox/types'

/**
 * How an outbound message is getting on.
 *
 * Sending is asynchronous, so a message exists in the mailbox before the
 * provider has accepted it. Without this the user cannot tell a message that is
 * still queued from one that failed hours ago.
 */
const STATUS: Record<
  string,
  { label: string; icon: typeof Clock; variant: 'default' | 'secondary' | 'destructive' | 'outline' }
> = {
  queued: { label: 'Sending', icon: Clock, variant: 'secondary' },
  sending: { label: 'Sending', icon: Clock, variant: 'secondary' },
  sent: { label: 'Sent', icon: Check, variant: 'outline' },
  delivered: { label: 'Delivered', icon: CheckCheck, variant: 'default' },
  bounced: { label: 'Bounced', icon: CircleAlert, variant: 'destructive' },
  failed: { label: 'Failed', icon: CircleAlert, variant: 'destructive' },
}

export function SendStatus({ message }: { message: Message }) {
  // Inbound mail has no send state to report.
  if (message.direction !== 'outbound') return null

  const entry = STATUS[message.status]
  if (!entry) return null

  return (
    <Badge variant={entry.variant} className="gap-1" data-testid="send-status">
      <entry.icon className="size-3" />
      {entry.label}
    </Badge>
  )
}
