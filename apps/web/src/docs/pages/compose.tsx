import { Callout, Cell, Code, H2, H3, Lede, P, Row, Table, UL } from '@/components/docs/prose'

export function ComposePage() {
  return (
    <>
      <Lede>
        Writing and sending mail, and what happens to a message after you press Send.
      </Lede>

      <H2 id="writing">Writing</H2>

      <P>
        <strong>Compose</strong> opens a window in the corner. It stays put while you move around
        the app, so looking something up does not cost you the draft.
      </P>

      <H3 id="sender">Choosing who it is from</H3>

      <P>
        The <strong>From</strong> picker lists the sender addresses on the account the draft
        belongs to. If it is empty, that account has no addresses yet — add one on a verified
        domain first.
      </P>

      <P>
        The sender determines which provider account actually sends, and therefore which
        reputation and which domain the message goes out under. It is not cosmetic.
      </P>

      <H3 id="recipients">Recipients</H3>

      <P>
        Type an address and press Enter, comma or Tab to commit it. Each becomes a chip you can
        remove. <strong>Cc</strong> and <strong>Bcc</strong> are hidden until you ask for them.
      </P>

      <H3 id="body">The body</H3>

      <P>
        The editor handles bold, italic, headings, lists, quotes and links. A plain-text version is
        produced alongside the HTML automatically, so recipients whose clients refuse HTML still
        get something readable.
      </P>

      <H3 id="autosave">Autosave</H3>

      <P>
        Drafts save on their own shortly after you stop typing, and the window tells you when it
        last succeeded. Closing it does not discard anything — the draft is in{' '}
        <strong>Drafts</strong>, and reopening it restores what you had.
      </P>

      <Callout tone="note" title="Drafts are not messages">
        <P>
          A draft lives in its own place until it is sent, which is why autosaving thirty times a
          minute does not churn your mailbox, and why a half-written message can never accidentally
          go out.
        </P>
      </Callout>

      <H2 id="attachments">Attachments</H2>

      <P>
        Attach files with the paperclip. Each uploads immediately and appears in the window with
        its size, so a failed upload is visible before you send rather than after.
      </P>

      <P>
        Attachment bytes go to object storage, never into the database. When you send, they are
        re-pointed from the draft to the message rather than copied — the same bytes, filed
        differently.
      </P>

      <Callout tone="security" title="Downloads are never rendered in place">
        <P>
          Every attachment downloads as a file, whatever it claims to be. An HTML or SVG attachment
          opened inline would run as a page on the same origin as your mailbox, so x-mail does not
          offer that at all.
        </P>
      </Callout>

      <H2 id="replying">Replying and forwarding</H2>

      <P>From an open conversation:</P>

      <UL>
        <li>
          <strong>Reply</strong> addresses the sender, keeps the subject and quotes the original.
        </li>
        <li>
          <strong>Reply all</strong> adds the other recipients — without adding you to your own
          message.
        </li>
        <li>
          <strong>Forward</strong> keeps the body and subject and leaves the recipients empty.
        </li>
      </UL>

      <P>
        A reply carries the headers that link it to the original, so mail clients at the other end
        thread it correctly rather than starting a new conversation.
      </P>

      <H2 id="sending">What happens when you send</H2>

      <P>
        Sending returns almost instantly, and the message is not on its way yet. That is
        deliberate, and it is worth understanding because the interface shows you the difference.
      </P>

      <UL>
        <li>x-mail validates the draft and turns it into a message in your mailbox.</li>
        <li>It queues a job and answers you. The compose window closes.</li>
        <li>
          A background worker picks the job up, decrypts that account's provider key and makes the
          call.
        </li>
        <li>Delivery events from the provider update the message afterwards.</li>
      </UL>

      <P>
        The alternative — holding the request open while calling the provider — means a slow
        provider becomes a slow interface, and a dropped connection leaves you unsure whether the
        message went.
      </P>

      <H3 id="status">Following a message</H3>

      <P>Outbound messages carry a badge in the conversation view:</P>

      <Table head={['Badge', 'Meaning']}>
        <Row>
          <Cell>Sending</Cell>
          <Cell>Queued, or being handed to the provider right now.</Cell>
        </Row>
        <Row>
          <Cell>Sent</Cell>
          <Cell>The provider accepted it.</Cell>
        </Row>
        <Row>
          <Cell>Delivered</Cell>
          <Cell>It reached the recipient's mail server.</Cell>
        </Row>
        <Row>
          <Cell>Bounced</Cell>
          <Cell>Rejected at the far end. The address may be wrong or closed.</Cell>
        </Row>
        <Row>
          <Cell>Failed</Cell>
          <Cell>
            It could not be sent at all — usually a rejected API key or an unverified domain.
          </Cell>
        </Row>
      </Table>

      <P>
        Status only ever moves forward. Providers deliver their events out of order, and a late
        “sent” must not undo a “delivered” you have already been shown.
      </P>

      <Callout tone="warning" title="Everything stuck on “Sending”">
        <P>
          The worker is not running. The API queues messages and the worker delivers them; without
          it, mail is accepted and then waits indefinitely. With Docker it starts automatically;
          running locally it needs <Code>make worker</Code>.
        </P>
      </Callout>

      <H2 id="duplicates">Why you cannot send twice</H2>

      <P>
        Double-clicking Send, or a worker crashing halfway through, are both ordinary events. x-mail
        blocks a duplicate at three separate points: claiming the draft only succeeds if it is
        still a draft, the queued job is keyed by the message, and the provider call carries an
        idempotency key generated once and reused on every retry.
      </P>

      <P>
        The third is the one that matters most — it is the only one that covers a worker dying
        after the provider had already accepted the message.
      </P>
    </>
  )
}
