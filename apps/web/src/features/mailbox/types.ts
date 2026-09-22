export interface Participant {
  name: string
  email: string
}

export interface Label {
  id: string
  mail_account_id: string
  name: string
  color: string
  /** Set for built-in labels (INBOX, SENT, …); null for user labels. */
  system_key: string | null
  created_at: string
  updated_at: string
}

export interface Thread {
  id: string
  mail_account_id: string
  subject: string
  snippet: string
  message_count: number
  unread_count: number
  has_attachments: boolean
  is_starred: boolean
  participants: Participant[]
  last_message_at: string
  created_at: string
  updated_at: string
  labels?: Label[]
  /** Which account the thread came from, for unified-inbox rows. */
  account_name?: string
  account_color?: string
}

export interface Recipient {
  id: string
  email: string
  name: string
  type: 'to' | 'cc' | 'bcc'
}

export interface Message {
  id: string
  mail_account_id: string
  thread_id: string
  message_id: string | null
  in_reply_to: string | null
  references: string[]
  provider_message_id: string | null
  sender_name: string
  sender_email: string
  subject: string
  snippet: string
  /** The body exactly as received; never rendered directly. */
  html_body: string
  /** Server-sanitised copy — this is what is safe to display. */
  html_sanitized: string
  text_body: string
  direction: 'inbound' | 'outbound'
  status: string
  is_read: boolean
  is_starred: boolean
  is_important: boolean
  has_attachments: boolean
  size_bytes: number
  sent_at: string | null
  received_at: string | null
  created_at: string
  updated_at: string
  recipients: Recipient[]
  labels: Label[]
}

export interface ThreadDetail {
  thread: Thread
  messages: Message[]
}

export interface ThreadPage {
  threads: Thread[]
  /** Empty when there are no further pages. */
  next_cursor: string
}

/** Bulk actions the toolbar can apply to a selection. */
export type ThreadAction =
  | 'read'
  | 'unread'
  | 'star'
  | 'unstar'
  | 'important'
  | 'unimportant'
  | 'archive'
  | 'inbox'
  | 'spam'
  | 'not_spam'
  | 'trash'
  | 'restore'
  | 'delete'
