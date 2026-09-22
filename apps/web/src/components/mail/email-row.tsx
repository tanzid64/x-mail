import { Link } from '@tanstack/react-router'
import { Paperclip, Star } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { accountDotClass } from '@/features/accounts/types'
import type { Thread } from '@/features/mailbox/types'
import { formatMailDate, summariseParticipants } from '@/lib/format'
import { cn } from '@/lib/utils'

interface EmailRowProps {
  thread: Thread
  selected: boolean
  onSelectedChange: (selected: boolean) => void
  onToggleStar: () => void
  /** Shown when the list spans more than one account. */
  showAccount: boolean
}

export function EmailRow({ thread, selected, onSelectedChange, onToggleStar, showAccount }: EmailRowProps) {
  const unread = thread.unread_count > 0

  return (
    <div
      data-testid="email-row"
      data-unread={unread}
      className={cn(
        'group flex items-center gap-3 border-b px-4 py-2 transition',
        unread ? 'bg-background font-medium' : 'bg-muted/30',
        selected ? 'bg-accent' : 'hover:bg-muted/60',
      )}
    >
      <Checkbox
        checked={selected}
        onCheckedChange={(value) => onSelectedChange(value === true)}
        aria-label={`Select “${thread.subject || '(no subject)'}”`}
      />

      <button
        type="button"
        onClick={onToggleStar}
        aria-label={thread.is_starred ? 'Unstar conversation' : 'Star conversation'}
        aria-pressed={thread.is_starred}
        className="shrink-0 text-muted-foreground transition hover:text-amber-500"
      >
        <Star className={cn('size-4', thread.is_starred && 'fill-amber-400 text-amber-500')} />
      </button>

      <Link
        to="/thread/$threadId"
        params={{ threadId: thread.id }}
        className="flex min-w-0 flex-1 items-center gap-3"
      >
        {showAccount && thread.account_name ? (
          <span className="flex shrink-0 items-center gap-1.5" title={thread.account_name}>
            <span className={cn('size-2 rounded-full', accountDotClass(thread.account_color ?? 'slate'))} />
            <span className="hidden text-xs text-muted-foreground lg:inline">{thread.account_name}</span>
          </span>
        ) : null}

        <span className="w-40 shrink-0 truncate text-sm">
          {summariseParticipants(thread.participants)}
          {thread.message_count > 1 ? (
            <span className="ml-1 text-xs text-muted-foreground">{thread.message_count}</span>
          ) : null}
        </span>

        <span className="min-w-0 flex-1 truncate text-sm">
          {thread.subject || '(no subject)'}
          {thread.snippet ? (
            <span className="ml-2 font-normal text-muted-foreground">— {thread.snippet}</span>
          ) : null}
        </span>

        {thread.labels?.map((label) => (
          <Badge key={label.id} variant="outline" className="hidden shrink-0 md:inline-flex">
            {label.name}
          </Badge>
        ))}

        {thread.has_attachments ? <Paperclip className="size-3.5 shrink-0 text-muted-foreground" /> : null}

        <span className="w-20 shrink-0 text-right text-xs text-muted-foreground">
          {formatMailDate(thread.last_message_at)}
        </span>
      </Link>
    </div>
  )
}
