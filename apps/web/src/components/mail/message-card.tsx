import { useState } from 'react'
import { ChevronDown, ChevronUp } from 'lucide-react'

import { MessageBody } from '@/components/mail/message-body'
import { SendStatus } from '@/components/mail/send-status'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import type { Message } from '@/features/mailbox/types'
import { displayName, formatFullDate, formatMailDate, initialsFor } from '@/lib/format'
import { cn } from '@/lib/utils'

interface MessageCardProps {
  message: Message
  /** Collapsed messages show only their header, as in a long thread. */
  defaultOpen: boolean
}

export function MessageCard({ message, defaultOpen }: MessageCardProps) {
  const [open, setOpen] = useState(defaultOpen)

  const sender = displayName(message.sender_name, message.sender_email)
  const to = message.recipients.filter((r) => r.type === 'to')
  const cc = message.recipients.filter((r) => r.type === 'cc')
  const timestamp = message.received_at ?? message.sent_at ?? message.created_at

  return (
    <article className={cn('rounded-lg border', open ? 'bg-card' : 'bg-muted/30')}>
      <header
        role="button"
        tabIndex={0}
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        onKeyDown={(event) => {
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault()
            setOpen((value) => !value)
          }
        }}
        className="flex cursor-pointer items-start gap-3 p-4"
      >
        <Avatar className="size-9 shrink-0">
          <AvatarFallback>{initialsFor(message.sender_name, message.sender_email)}</AvatarFallback>
        </Avatar>

        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium">{sender}</span>
            <span className="text-sm text-muted-foreground">{message.sender_email}</span>
            <SendStatus message={message} />
          </div>

          {open ? (
            <p className="mt-0.5 text-sm text-muted-foreground">
              to {to.map((r) => r.email).join(', ') || '(undisclosed)'}
              {cc.length > 0 ? ` · cc ${cc.map((r) => r.email).join(', ')}` : ''}
            </p>
          ) : (
            <p className="mt-0.5 truncate text-sm text-muted-foreground">{message.snippet}</p>
          )}
        </div>

        <div className="flex shrink-0 items-center gap-2">
          <time dateTime={timestamp} title={formatFullDate(timestamp)} className="text-xs text-muted-foreground">
            {formatMailDate(timestamp)}
          </time>
          {open ? (
            <ChevronUp className="size-4 text-muted-foreground" />
          ) : (
            <ChevronDown className="size-4 text-muted-foreground" />
          )}
        </div>
      </header>

      {open ? (
        <div className="border-t px-4 py-3">
          {/* Always the sanitised copy — the server guarantees one for every
              stored message, inbound or outbound. */}
          <MessageBody html={message.html_sanitized} text={message.text_body} />
        </div>
      ) : null}
    </article>
  )
}
