export interface ComposeAddress {
  email: string
  name?: string
}

export interface Attachment {
  id: string
  message_id: string | null
  draft_id: string | null
  filename: string
  mime_type: string
  size_bytes: number
  content_id: string | null
  is_inline: boolean
  created_at: string
}

export interface Draft {
  id: string
  mail_account_id: string
  thread_id: string | null
  in_reply_to_id: string | null
  from_address_id: string | null
  to: ComposeAddress[]
  cc: ComposeAddress[]
  bcc: ComposeAddress[]
  subject: string
  html_body: string
  text_body: string
  status: 'draft' | 'sending'
  created_at: string
  updated_at: string
  attachments: Attachment[]
}

/** What the compose window holds while it is open. */
export interface ComposeFields {
  from_address_id: string | null
  to: ComposeAddress[]
  cc: ComposeAddress[]
  bcc: ComposeAddress[]
  subject: string
  html_body: string
  text_body: string
}
