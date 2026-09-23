import { Endpoint, Field, Fields, Part } from '@/components/docs/endpoint'
import { Callout, Cell, Code, CodeBlock, H2, Lede, P, Row, Table } from '@/components/docs/prose'

export function ApiMailboxPage() {
  return (
    <>
      <Lede>Reading conversations, acting on them, and the labels they carry.</Lede>

      <H2 id="threads">Conversations</H2>

      <Endpoint id="list-threads" method="GET" path="/threads">
        <P>One page of conversations.</P>
        <Part title="Query">
          <Fields>
            <Field name="folder" type="string">
              <Code>inbox</Code> (default), <Code>starred</Code>, <Code>snoozed</Code>,{' '}
              <Code>sent</Code>, <Code>drafts</Code>, <Code>archive</Code>, <Code>spam</Code>,{' '}
              <Code>trash</Code> or <Code>important</Code>. An unknown value is <Code>400</Code>.
            </Field>
            <Field name="mail_account_id" type="uuid">
              Omit for the unified view across every account you own.
            </Field>
            <Field name="label_id" type="uuid">
              Narrow to one label.
            </Field>
            <Field name="limit" type="integer">
              Defaults to 50. Must be positive.
            </Field>
            <Field name="cursor" type="string">
              The <Code>next_cursor</Code> from the previous page.
            </Field>
            <Field name="unread" type="string">
              Send <Code>true</Code> for only conversations with unread mail.
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <CodeBlock>{`{
  "threads": [
    {
      "id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
      "mail_account_id": "8f14e45f-ceea-467a-9b8f-6f2d3c4a5b6c",
      "subject": "Your order shipped",
      "snippet": "On its way — tracking below.",
      "message_count": 3,
      "unread_count": 1,
      "has_attachments": false,
      "is_starred": false,
      "participants": [{ "name": "Ada", "email": "ada@example.com" }],
      "last_message_at": "2026-01-12T09:30:00Z",
      "labels": [],
      "account_name": "Support",
      "account_color": "emerald",
      "created_at": "2026-01-11T14:00:00Z",
      "updated_at": "2026-01-12T09:30:00Z"
    }
  ],
  "next_cursor": "eyJsYXN0IjoiMjAyNi0wMS0xMiJ9"
}`}</CodeBlock>
          <P>
            <Code>account_name</Code> and <Code>account_color</Code> appear on unified-inbox rows,
            so a client need not join accounts itself. An empty <Code>next_cursor</Code> is the
            last page.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="thread-counts" method="GET" path="/threads/counts">
        <P>Unread count per folder — what the sidebar badges show.</P>
        <Part title="Query">
          <Fields>
            <Field name="mail_account_id" type="uuid">
              Narrow to one account.
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <CodeBlock>{`{ "counts": { "inbox": 12, "starred": 3, "spam": 0 } }`}</CodeBlock>
        </Part>
      </Endpoint>

      <Endpoint id="get-thread" method="GET" path="/threads/{id}">
        <P>A conversation and its messages, oldest first.</P>
        <Part title="Query">
          <Fields>
            <Field name="mark_read" type="string">
              Send <Code>false</Code> to look without marking it read. Opening a thread marks it
              read otherwise, which is what opening a thread means in a mail client.
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <CodeBlock>{`{
  "thread": { ... },
  "messages": [
    {
      "id": "16fd2706-8baf-433b-82eb-8c7fada847da",
      "thread_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
      "message_id": "<abc@mail.example.com>",
      "in_reply_to": null,
      "references": [],
      "sender_name": "Ada",
      "sender_email": "ada@example.com",
      "subject": "Your order shipped",
      "snippet": "On its way — tracking below.",
      "html_body": "<p>On its way…</p>",
      "html_sanitized": "<p>On its way…</p>",
      "text_body": "On its way…",
      "direction": "inbound",
      "status": "received",
      "is_read": true,
      "is_starred": false,
      "is_important": false,
      "has_attachments": false,
      "size_bytes": 4821,
      "sent_at": null,
      "received_at": "2026-01-12T09:30:00Z",
      "recipients": [
        { "id": "…", "email": "support@company.com", "name": "", "type": "to" }
      ],
      "labels": [],
      "created_at": "2026-01-12T09:30:01Z",
      "updated_at": "2026-01-12T09:30:01Z"
    }
  ]
}`}</CodeBlock>
        </Part>
        <Part title="Notes">
          <Callout tone="security" title="Two bodies, and only one is safe to render">
            <P>
              <Code>html_sanitized</Code> is the render-safe copy, cleaned when the message
              arrived. <Code>html_body</Code> is the original exactly as received and is{' '}
              <strong>untrusted</strong> — it is kept so a future sanitiser fix can be re-applied,
              not so it can be displayed. Never inject it into a page, and render the sanitised
              copy in a sandboxed frame with scripts disabled.
            </P>
          </Callout>
          <P>
            <Code>status</Code> for outbound mail moves <Code>queued</Code> → <Code>sending</Code>{' '}
            → <Code>sent</Code> → <Code>delivered</Code>, or ends at <Code>bounced</Code> or{' '}
            <Code>failed</Code>. Inbound mail is <Code>received</Code>. Status only ever moves
            forward: providers reorder their events, and a late “sent” must not undo a recorded
            “delivered”.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="thread-actions" method="POST" path="/threads/actions">
        <P>Applies one action to many conversations. This is the only way to change them.</P>
        <Part title="Body">
          <Fields>
            <Field name="action" type="string" required>
              See the table below.
            </Field>
            <Field name="thread_ids" type="uuid[]" required>
              Between 1 and 500.
            </Field>
            <Field name="mail_account_id" type="uuid">
              Not used — scope comes from the ids themselves.
            </Field>
            <Field name="label_id" type="uuid">
              Required for <Code>label</Code> and <Code>unlabel</Code>, ignored otherwise.
            </Field>
          </Fields>
        </Part>
        <Part title="Actions">
          <Table head={['Action', 'Effect']}>
            <Row>
              <Cell mono>read / unread</Cell>
              <Cell>Marks every message in the conversation.</Cell>
            </Row>
            <Row>
              <Cell mono>star / unstar</Cell>
              <Cell>Puts it in Starred, or takes it out.</Cell>
            </Row>
            <Row>
              <Cell mono>important / unimportant</Cell>
              <Cell>Sets the important flag.</Cell>
            </Row>
            <Row>
              <Cell mono>archive</Cell>
              <Cell>Out of the inbox, still in the mailbox.</Cell>
            </Row>
            <Row>
              <Cell mono>inbox</Cell>
              <Cell>Back to the inbox from archive, spam or trash.</Cell>
            </Row>
            <Row>
              <Cell mono>spam / not_spam</Cell>
              <Cell>Files under spam, or takes it back out.</Cell>
            </Row>
            <Row>
              <Cell mono>trash</Cell>
              <Cell>Deleted but recoverable.</Cell>
            </Row>
            <Row>
              <Cell mono>restore</Cell>
              <Cell>Back out of trash.</Cell>
            </Row>
            <Row>
              <Cell mono>delete</Cell>
              <Cell>Permanent. No undo.</Cell>
            </Row>
            <Row>
              <Cell mono>label / unlabel</Cell>
              <Cell>
                Applies or removes <Code>label_id</Code>.
              </Cell>
            </Row>
          </Table>
        </Part>
        <Part title="Returns">
          <CodeBlock>{`{ "affected": 12 }`}</CodeBlock>
          <P>
            Conversations you do not own are skipped silently rather than reported, so{' '}
            <Code>affected</Code> cannot be used to probe for other people's mail. A count lower
            than the number of ids you sent is not an error.
          </P>
        </Part>
      </Endpoint>

      <H2 id="labels">Labels</H2>

      <P>
        Every account gets system labels — <Code>INBOX</Code>, <Code>SENT</Code>,{' '}
        <Code>DRAFT</Code>, <Code>ARCHIVE</Code>, <Code>SPAM</Code>, <Code>TRASH</Code> — which
        decide where a conversation is filed. Your own labels sit alongside them and are
        independent of placement, so a conversation can be in the inbox and carry three labels at
        once.
      </P>

      <Callout tone="note" title="No management screen yet">
        <P>
          These endpoints work. The interface has no label management screen, so today they are
          API-only.
        </P>
      </Callout>

      <Endpoint id="list-labels" method="GET" path="/labels">
        <P>System and user labels.</P>
        <Part title="Query">
          <Fields>
            <Field name="mail_account_id" type="uuid">
              Narrow to one account.
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <CodeBlock>{`{
  "labels": [
    {
      "id": "…", "mail_account_id": "…",
      "name": "Inbox", "color": "slate",
      "system_key": "INBOX",
      "created_at": "…", "updated_at": "…"
    },
    {
      "id": "…", "mail_account_id": "…",
      "name": "Invoices", "color": "amber",
      "system_key": null,
      "created_at": "…", "updated_at": "…"
    }
  ]
}`}</CodeBlock>
          <P>
            <Code>system_key</Code> is non-null for built-in labels, which cannot be renamed or
            deleted.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="create-label" method="POST" path="/labels">
        <P>Creates a label on one account.</P>
        <Part title="Body">
          <Fields>
            <Field name="mail_account_id" type="uuid" required>
              Labels belong to an account, not to you.
            </Field>
            <Field name="name" type="string" required>
              Up to 100 characters, unique within the account.
            </Field>
            <Field name="color" type="string">
              —
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <P>
            <Code>201</Code>, or <Code>409</Code> if the name is taken on that account.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="patch-label" method="PATCH" path="/labels/{id}">
        <P>
          Renames or recolours. Renaming a system label is refused with <Code>422</Code>.
        </P>
        <Part title="Body">
          <Fields>
            <Field name="name" type="string">
              —
            </Field>
            <Field name="color" type="string">
              —
            </Field>
          </Fields>
        </Part>
      </Endpoint>

      <Endpoint id="delete-label" method="DELETE" path="/labels/{id}">
        <P>
          Deletes a label. <Code>204</Code>. The conversations that carried it are untouched;
          deleting a system label is refused with <Code>422</Code>.
        </P>
      </Endpoint>
    </>
  )
}
