import { useCallback } from 'react'

import { useAddresses, useMailAccounts } from '@/features/accounts/api'
import { useCreateDraft } from '@/features/compose/api'
import type { ComposeAddress } from '@/features/compose/types'
import type { Message } from '@/features/mailbox/types'
import { useComposeStore } from '@/stores/compose'
import { useUIStore } from '@/stores/ui'

interface StartOptions {
  to?: ComposeAddress[]
  cc?: ComposeAddress[]
  subject?: string
  htmlBody?: string
  threadId?: string | null
  inReplyToId?: string | null
  /** Which account to compose from; defaults to the active or default account. */
  mailAccountId?: string
}

/**
 * Creates a draft on the server and opens the compose window on it.
 *
 * The draft exists before the window does, so anything typed is autosaved
 * against a real row from the first keystroke rather than being held in memory
 * until the first save.
 */
export function useStartCompose() {
  const { data: accounts } = useMailAccounts()
  const { data: addresses } = useAddresses()
  const createDraft = useCreateDraft()
  const open = useComposeStore((state) => state.open)
  const activeAccount = useUIStore((state) => state.activeAccount)

  const canCompose = (accounts?.length ?? 0) > 0

  const start = useCallback(
    async (options: StartOptions = {}) => {
      const list = accounts ?? []
      if (list.length === 0) return null

      // Prefer the account the caller named, then the one being viewed, then
      // the user's default.
      const accountId =
        options.mailAccountId ??
        (activeAccount !== 'all' ? activeAccount : undefined) ??
        list.find((account) => account.is_default)?.id ??
        list[0]!.id

      const candidates = (addresses ?? []).filter(
        (address) => address.mail_account_id === accountId && address.is_active,
      )
      const from = candidates.find((address) => address.is_default) ?? candidates[0]

      const draft = await createDraft.mutateAsync({
        mail_account_id: accountId,
        from_address_id: from?.id ?? null,
        thread_id: options.threadId ?? null,
        in_reply_to_id: options.inReplyToId ?? null,
        to: options.to ?? [],
        cc: options.cc ?? [],
        subject: options.subject ?? '',
        html_body: options.htmlBody ?? '',
      })

      open(draft.id)
      return draft
    },
    [accounts, addresses, activeAccount, createDraft, open],
  )

  return { start, canCompose, isPending: createDraft.isPending }
}

/** Prefixes a subject for a reply, without stacking "Re:" forever. */
export function replySubject(subject: string): string {
  return /^re:\s/i.test(subject) ? subject : `Re: ${subject}`
}

export function forwardSubject(subject: string): string {
  return /^fwd?:\s/i.test(subject) ? subject : `Fwd: ${subject}`
}

/**
 * The quoted original, as every mail client includes it.
 *
 * The body is inserted as-is because it is either mail we composed or mail the
 * server has already sanitised; it is still only ever rendered inside the
 * sandboxed message frame.
 */
export function quoteMessage(message: Message): string {
  const when = message.received_at ?? message.sent_at ?? message.created_at
  const date = new Date(when).toLocaleString()
  const who = message.sender_name ? `${message.sender_name} <${message.sender_email}>` : message.sender_email
  const body = message.html_body || `<p>${escapeHTML(message.text_body)}</p>`

  return `<p></p><p>On ${escapeHTML(date)}, ${escapeHTML(who)} wrote:</p><blockquote>${body}</blockquote>`
}

/** Recipients for "reply all": everyone except the addresses we own. */
export function replyAllRecipients(
  message: Message,
  ownAddresses: string[],
): { to: ComposeAddress[]; cc: ComposeAddress[] } {
  const own = new Set(ownAddresses.map((address) => address.toLowerCase()))

  const to: ComposeAddress[] = []
  if (!own.has(message.sender_email.toLowerCase())) {
    to.push({ email: message.sender_email, name: message.sender_name })
  }

  const cc: ComposeAddress[] = []
  const seen = new Set([...own, message.sender_email.toLowerCase()])
  for (const recipient of message.recipients) {
    if (recipient.type === 'bcc') continue
    const email = recipient.email.toLowerCase()
    if (seen.has(email)) continue
    seen.add(email)
    cc.push({ email: recipient.email, name: recipient.name })
  }

  // Replying to your own sent message keeps its original recipients.
  if (to.length === 0 && cc.length > 0) {
    return { to: cc, cc: [] }
  }
  return { to, cc }
}

function escapeHTML(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
}
