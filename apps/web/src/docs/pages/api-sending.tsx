import { Endpoint, Field, Fields, Part } from '@/components/docs/endpoint'
import { Callout, Code, CodeBlock, H2, Lede, P, UL } from '@/components/docs/prose'

export function ApiSendingPage() {
  return (
    <>
      <Lede>
        Sending is two calls: build a draft, then send it. There is no single-shot send endpoint,
        and the split is deliberate.
      </Lede>

      <P>
        A draft can be built up over several requests, have attachments added to it, and be
        inspected before it goes. It also means the thing that gets sent is a row that already
        exists, which is what makes the send idempotent — a retry claims the same draft rather
        than composing a second message.
      </P>

      <H2 id="drafts">Drafts</H2>

      <Endpoint id="list-drafts" method="GET" path="/drafts">
        <P>Your drafts, most recently edited first.</P>
        <Part title="Query">
          <Fields>
            <Field name="mail_account_id" type="uuid">
              Narrow to one account.
            </Field>
          </Fields>
        </Part>
      </Endpoint>

      <Endpoint id="create-draft" method="POST" path="/drafts">
        <P>Creates a draft. Only the account is required; everything else can come later.</P>
        <Part title="Body">
          <Fields>
            <Field name="mail_account_id" type="uuid" required>
              Which mailbox this belongs to.
            </Field>
            <Field name="from_address_id" type="uuid | null">
              Must be an address on that account. Required before it can be sent, but not to
              create it.
            </Field>
            <Field name="to" type="address[]">
              <Code>{'[{ "email": "…", "name": "…" }]'}</Code>. <Code>name</Code> is optional.
            </Field>
            <Field name="cc" type="address[]">
              —
            </Field>
            <Field name="bcc" type="address[]">
              —
            </Field>
            <Field name="subject" type="string">
              —
            </Field>
            <Field name="html_body" type="string">
              —
            </Field>
            <Field name="text_body" type="string">
              The plain-text alternative, for clients that will not render HTML.
            </Field>
            <Field name="in_reply_to_id" type="uuid | null">
              The message being replied to. Sets the threading headers so the recipient's client
              threads it correctly rather than starting a new conversation.
            </Field>
            <Field name="thread_id" type="uuid | null">
              The conversation to file it under. Usually implied by{' '}
              <Code>in_reply_to_id</Code>.
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <CodeBlock>{`{
  "draft": {
    "id": "0d5f0f9e-8a1f-4a1f-9b6a-2c2c7f4e1a11",
    "mail_account_id": "8f14e45f-ceea-467a-9b8f-6f2d3c4a5b6c",
    "from_address_id": "c9bf9e57-1685-4c89-bafb-ff5af830be8a",
    "thread_id": null,
    "in_reply_to_id": null,
    "to": [{ "email": "customer@example.com" }],
    "cc": [], "bcc": [],
    "subject": "Your order shipped",
    "html_body": "<p>On its way.</p>",
    "text_body": "On its way.",
    "status": "draft",
    "attachments": [],
    "created_at": "…", "updated_at": "…"
  }
}`}</CodeBlock>
          <P>
            <Code>201</Code>, or <Code>404</Code> if the account is not yours.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="get-draft" method="GET" path="/drafts/{id}">
        <P>One draft, with its attachments.</P>
      </Endpoint>

      <Endpoint id="patch-draft" method="PATCH" path="/drafts/{id}">
        <P>
          Updates a draft. Omitted fields are left alone, so this is safe to call repeatedly — it
          is what the interface's autosave does.
        </P>
        <Part title="Body">
          <P>
            The same fields as creating, minus <Code>mail_account_id</Code>: a draft cannot change
            which mailbox it belongs to.
          </P>
        </Part>
        <Part title="Returns">
          <P>
            <Code>200</Code> with the updated draft.
          </P>
          <P>
            <Code>409</Code> once a send has claimed it. After that point, edits cannot change what
            goes out — which is the property that stops a late autosave from altering a message
            mid-flight.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="delete-draft" method="DELETE" path="/drafts/{id}">
        <P>
          Discards a draft and removes its attachments from storage. <Code>204</Code>.
        </P>
      </Endpoint>

      <H2 id="sending">Sending</H2>

      <Endpoint id="send-draft" method="POST" path="/drafts/{id}/send">
        <P>Turns the draft into a message and queues it for delivery.</P>
        <Part title="Returns">
          <CodeBlock>{`{
  "message_id": "16fd2706-8baf-433b-82eb-8c7fada847da",
  "thread_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7"
}`}</CodeBlock>
          <Callout tone="note" title="202, not 200">
            <P>
              The provider has not been called yet. x-mail has validated the draft, written the
              message and queued a job; a worker makes the call. Follow the message's{' '}
              <Code>status</Code> via <Code>GET /threads/{'{thread_id}'}</Code>, or listen on the
              websocket.
            </P>
          </Callout>
          <P>Failures worth handling:</P>
          <UL>
            <li>
              <Code>422</Code> — nothing to send to, no sender address set, or a sender whose
              domain is not verified.
            </li>
            <li>
              <Code>409</Code> — a send is already under way for this draft.
            </li>
            <li>
              <Code>404</Code> — not yours, or already sent.
            </li>
          </UL>
        </Part>
        <Part title="Idempotency">
          <P>
            Calling this twice does not send twice. Claiming the draft only succeeds while it is
            still a draft, the queued job is keyed by the message, and the provider call carries an
            idempotency key generated once and reused on every retry. The last of those is the one
            that matters — it is the only one that covers a worker dying after the provider had
            already accepted the message.
          </P>
        </Part>
      </Endpoint>

      <H2 id="attachments">Attachments</H2>

      <Endpoint id="upload-attachment" method="POST" path="/drafts/{id}/attachments">
        <P>
          Attaches a file. <Code>multipart/form-data</Code>, with the file in a part named{' '}
          <Code>file</Code>.
        </P>
        <Part title="Example">
          <CodeBlock>{`curl -X POST "$HOST/drafts/$DRAFT/attachments" \\
  -H "Authorization: Bearer $TOKEN" \\
  -F "file=@invoice.pdf"`}</CodeBlock>
        </Part>
        <Part title="Returns">
          <CodeBlock>{`{
  "attachment": {
    "id": "b3f1c6d2-3a5e-4e6b-9f2c-1d0a7b8e9f01",
    "draft_id": "0d5f0f9e-8a1f-4a1f-9b6a-2c2c7f4e1a11",
    "message_id": null,
    "filename": "invoice.pdf",
    "mime_type": "application/pdf",
    "size_bytes": 84213,
    "content_id": null,
    "is_inline": false,
    "created_at": "…"
  }
}`}</CodeBlock>
          <P>
            <Code>201</Code>, or <Code>413</Code> if the file is over the limit.
          </P>
          <P>
            The bytes go to object storage; the row holds a key. When the draft is sent they are
            re-pointed at the message rather than copied, so sending never doubles what you are
            storing.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="download-attachment" method="GET" path="/attachments/{id}">
        <P>Downloads the file. Works for attachments on drafts and on received messages.</P>
        <Part title="Returns">
          <P>
            The bytes, always as <Code>application/octet-stream</Code> with{' '}
            <Code>Content-Disposition: attachment</Code> and <Code>X-Content-Type-Options: nosniff</Code>
            , whatever the file claims to be.
          </P>
          <Callout tone="security" title="Never served inline">
            <P>
              An HTML or SVG attachment rendered in place would execute as a page on the same
              origin as your mailbox. Serving every attachment as a download removes that
              possibility rather than trying to detect the dangerous ones.
            </P>
          </Callout>
        </Part>
      </Endpoint>

      <Endpoint id="delete-attachment" method="DELETE" path="/attachments/{id}">
        <P>
          Removes an attachment from a draft and deletes the stored bytes. <Code>204</Code>.
        </P>
      </Endpoint>
    </>
  )
}
