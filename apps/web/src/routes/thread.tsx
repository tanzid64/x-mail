import { Link, useNavigate, useParams } from '@tanstack/react-router'
import { Archive, ArrowLeft, Forward, Reply, ReplyAll, ShieldAlert, Star, Trash2 } from 'lucide-react'

import { MessageCard } from '@/components/mail/message-card'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { useAddresses } from '@/features/accounts/api'
import {
  forwardSubject,
  quoteMessage,
  replyAllRecipients,
  replySubject,
  useStartCompose,
} from '@/features/compose/use-start-compose'
import { useThread, useThreadAction } from '@/features/mailbox/api'
import type { ThreadAction } from '@/features/mailbox/types'
import { INBOX } from '@/router'
import { cn } from '@/lib/utils'

export function ThreadPage() {
  const { threadId } = useParams({ from: '/_app/thread/$threadId' })
  const navigate = useNavigate()
  const { data, isLoading, error } = useThread(threadId)
  const act = useThreadAction()
  const { data: addresses } = useAddresses()
  const { start, canCompose } = useStartCompose()

  if (isLoading) {
    return (
      <div className="mx-auto flex w-full max-w-4xl flex-col gap-3 p-6">
        <Skeleton className="h-8 w-2/3" />
        <Skeleton className="h-40 w-full" />
      </div>
    )
  }

  if (error || !data) {
    return (
      <div className="mx-auto flex w-full max-w-4xl flex-col items-start gap-3 p-6">
        <p className="text-sm text-muted-foreground">
          {error?.status === 404 ? 'That conversation does not exist.' : error?.message}
        </p>
        <Button asChild variant="outline">
          <Link {...INBOX}>Back to inbox</Link>
        </Button>
      </div>
    )
  }

  const { thread, messages } = data
  const lastIndex = messages.length - 1
  const latest = messages[lastIndex]

  // Every address this user owns, so "reply all" does not add them to their own
  // recipient list.
  const ownAddresses = (addresses ?? []).map((address) => address.email)

  const reply = (all: boolean) => {
    if (!latest) return
    const recipients = all
      ? replyAllRecipients(latest, ownAddresses)
      : { to: [{ email: latest.sender_email, name: latest.sender_name }], cc: [] }

    void start({
      ...recipients,
      subject: replySubject(thread.subject),
      htmlBody: quoteMessage(latest),
      threadId: thread.id,
      inReplyToId: latest.id,
      mailAccountId: thread.mail_account_id,
    })
  }

  const forward = () => {
    if (!latest) return
    void start({
      subject: forwardSubject(thread.subject),
      htmlBody: quoteMessage(latest),
      // A forward starts a new conversation rather than continuing this one.
      mailAccountId: thread.mail_account_id,
    })
  }

  // Actions that remove the thread from view should return to the mailbox.
  const runAndLeave = (action: ThreadAction) => {
    act.mutate({ action, threadIds: [threadId] }, { onSuccess: () => void navigate(INBOX) })
  }

  return (
    <div className="mx-auto flex w-full max-w-4xl flex-col gap-4 p-6">
      <div className="flex flex-wrap items-center gap-1">
        <Button asChild variant="ghost" size="icon" aria-label="Back to inbox">
          <Link {...INBOX}>
            <ArrowLeft className="size-4" />
          </Link>
        </Button>

        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              aria-label={thread.is_starred ? 'Unstar conversation' : 'Star conversation'}
              aria-pressed={thread.is_starred}
              onClick={() => act.mutate({ action: thread.is_starred ? 'unstar' : 'star', threadIds: [threadId] })}
            >
              <Star className={cn('size-4', thread.is_starred && 'fill-amber-400 text-amber-500')} />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{thread.is_starred ? 'Unstar' : 'Star'}</TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="icon" aria-label="Archive" onClick={() => runAndLeave('archive')}>
              <Archive className="size-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Archive</TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="icon" aria-label="Report spam" onClick={() => runAndLeave('spam')}>
              <ShieldAlert className="size-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Report spam</TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="icon" aria-label="Move to trash" onClick={() => runAndLeave('trash')}>
              <Trash2 className="size-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Move to trash</TooltipContent>
        </Tooltip>
      </div>

      <div>
        <h1 className="text-xl font-semibold">{thread.subject || '(no subject)'}</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          {thread.message_count} message{thread.message_count === 1 ? '' : 's'}
          {thread.account_name ? ` · ${thread.account_name}` : ''}
        </p>
      </div>

      <div className="flex flex-col gap-2">
        {messages.map((message, index) => (
          // The newest message is what you came to read; earlier ones collapse.
          <MessageCard key={message.id} message={message} defaultOpen={index === lastIndex} />
        ))}
      </div>

      <div className="flex flex-wrap gap-2">
        <Button variant="outline" className="gap-2" disabled={!canCompose} onClick={() => reply(false)}>
          <Reply className="size-4" />
          Reply
        </Button>
        <Button variant="outline" className="gap-2" disabled={!canCompose} onClick={() => reply(true)}>
          <ReplyAll className="size-4" />
          Reply all
        </Button>
        <Button variant="outline" className="gap-2" disabled={!canCompose} onClick={forward}>
          <Forward className="size-4" />
          Forward
        </Button>
      </div>
    </div>
  )
}
