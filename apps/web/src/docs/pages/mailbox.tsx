import { Callout, Cell, Code, H2, H3, Lede, P, Row, Table, UL } from '@/components/docs/prose'

export function MailboxPage() {
  return (
    <>
      <Lede>
        The inbox works the way you expect a mail client to work. This page covers the parts where
        x-mail differs, and the ones worth knowing about.
      </Lede>

      <H2 id="folders">Folders</H2>

      <P>
        The sidebar has the usual set. Each is a view over the same messages rather than a place
        they are physically kept, which is why a conversation can leave one and appear in another
        instantly.
      </P>

      <Table head={['Folder', 'What is in it']}>
        <Row>
          <Cell>Inbox</Cell>
          <Cell>Mail that has arrived and has not been archived, trashed or marked spam.</Cell>
        </Row>
        <Row>
          <Cell>Starred</Cell>
          <Cell>Anything you starred, wherever it otherwise lives.</Cell>
        </Row>
        <Row>
          <Cell>Snoozed</Cell>
          <Cell>Reserved for scheduled re-delivery. Not built yet.</Cell>
        </Row>
        <Row>
          <Cell>Sent</Cell>
          <Cell>Mail you sent, including anything still on its way out.</Cell>
        </Row>
        <Row>
          <Cell>Drafts</Cell>
          <Cell>Messages you have started and not sent.</Cell>
        </Row>
        <Row>
          <Cell>Archive</Cell>
          <Cell>Out of the inbox, still in the mailbox.</Cell>
        </Row>
        <Row>
          <Cell>Spam</Cell>
          <Cell>What you have reported as spam.</Cell>
        </Row>
        <Row>
          <Cell>Trash</Cell>
          <Cell>Deleted but still recoverable.</Cell>
        </Row>
      </Table>

      <P>
        The number next to a folder is unread conversations, not total ones, and it moves on its
        own as mail arrives.
      </P>

      <H2 id="unified">One account, or all of them</H2>

      <P>
        With <strong>All accounts</strong> selected, every folder reads across every mailbox you
        own at once. Rows carry a coloured dot and the account name so you can see where each one
        came from.
      </P>

      <P>
        Picking a single account narrows every folder to it. Nothing is merged and nothing is
        copied — the unified view is a query across accounts, and each message stays filed under
        the account that received it.
      </P>

      <H2 id="reading">Reading</H2>

      <P>
        Clicking a row opens the conversation with its messages oldest-first, and marks it read.
        The list behind you updates immediately.
      </P>

      <H3 id="rendering">How message bodies are rendered</H3>

      <P>
        Incoming HTML is hostile until proven otherwise, and x-mail treats it that way twice over.
      </P>

      <UL>
        <li>
          It is <strong>sanitised when it arrives</strong>, not when you open it. Scripts, event
          handlers and dangerous URLs are stripped once and the safe copy is stored alongside the
          untouched original.
        </li>
        <li>
          It is <strong>rendered in a sandboxed frame with scripts disabled</strong>. Even if
          something got through the sanitiser, it has no JavaScript to run and no access to your
          session.
        </li>
      </UL>

      <P>
        Links open in a new tab. The frame resizes itself to its content, so there is no scrollbar
        inside a scrollbar.
      </P>

      <Callout tone="note" title="Why a message might look plainer than expected">
        <P>
          Senders sometimes rely on markup the sanitiser removes. The text is all there; some of
          the styling may not be. That is the trade being made deliberately.
        </P>
      </Callout>

      <H2 id="organising">Organising</H2>

      <P>
        Select conversations with the checkboxes — or the one in the toolbar for everything in
        view — and the toolbar offers what makes sense for where you are. Archiving from the
        archive is not offered, because it would do nothing.
      </P>

      <Table head={['Action', 'Available in', 'What it does']}>
        <Row>
          <Cell>Archive</Cell>
          <Cell>Inbox and elsewhere</Cell>
          <Cell>Out of the inbox, kept in the mailbox.</Cell>
        </Row>
        <Row>
          <Cell>Move to inbox</Cell>
          <Cell>Archive, spam, trash</Cell>
          <Cell>Brings it back.</Cell>
        </Row>
        <Row>
          <Cell>Report spam</Cell>
          <Cell>Everywhere but spam and trash</Cell>
          <Cell>Files it under spam.</Cell>
        </Row>
        <Row>
          <Cell>Not spam</Cell>
          <Cell>Spam</Cell>
          <Cell>Takes it back out.</Cell>
        </Row>
        <Row>
          <Cell>Move to trash</Cell>
          <Cell>Everywhere but trash</Cell>
          <Cell>Deleted, still recoverable.</Cell>
        </Row>
        <Row>
          <Cell>Delete forever</Cell>
          <Cell>Trash only</Cell>
          <Cell>Gone. No undo.</Cell>
        </Row>
        <Row>
          <Cell>Mark read / unread</Cell>
          <Cell>Everywhere</Cell>
          <Cell />
        </Row>
      </Table>

      <P>
        Stars are per conversation and set from the row itself — no selection needed. A starred
        conversation appears in Starred regardless of which folder it is filed in.
      </P>

      <Callout tone="tip" title="Bulk actions are one request">
        <P>
          Selecting fifty conversations and archiving them sends a single request, not fifty. Up to
          500 at a time.
        </P>
      </Callout>

      <H2 id="realtime">Things move on their own</H2>

      <P>
        x-mail holds a websocket while the app is open. When mail arrives, when a message you sent
        is delivered, or when you act on something in another tab, the affected views refresh
        themselves.
      </P>

      <P>
        Those events carry identifiers only — never message content. The browser refetches through
        the same authorised endpoints it always uses, so the live channel cannot become a way for
        mail to reach someone who should not see it.
      </P>

      <P>
        If the connection drops it reconnects on its own. <strong>Refresh</strong> in the toolbar
        is there for when you want to be certain rather than because you should need it.
      </P>

      <H2 id="pagination">Long mailboxes</H2>

      <P>
        The list pages as you reach the end rather than numbering pages. A mailbox changes while
        you are reading it, and numbered pages silently skip or repeat conversations as new mail
        arrives at the top; paging from where you actually are does not.
      </P>

      <H2 id="not-yet">Not built yet</H2>

      <P>
        Worth stating plainly so you are not hunting for them: <strong>search</strong>,{' '}
        <strong>custom labels in the interface</strong>, <strong>snooze</strong>,{' '}
        <strong>keyboard shortcuts</strong>, <strong>signatures</strong> and{' '}
        <strong>filters</strong> are not implemented. Labels exist in the data model and over the
        API; they have no management screen yet. The <Code>Snoozed</Code> folder is present and
        always empty.
      </P>
    </>
  )
}
