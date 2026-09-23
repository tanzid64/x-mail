import { Link } from '@tanstack/react-router'

import { Callout, Code, CodeBlock, H2, Lede, P, UL } from '@/components/docs/prose'

export function OverviewPage() {
  return (
    <>
      <Lede>
        x-mail is a mail platform you run yourself. It gives one person a single inbox across
        several sending accounts and several domains, and exposes every one of those capabilities
        over an API.
      </Lede>

      <P>
        It is not a mail server. It does not speak SMTP to the outside world and it does not try
        to. Delivery and receipt are handled by a provider — Resend today — while x-mail owns
        everything that makes a mailbox a mailbox: users, accounts, addresses, threads, messages,
        drafts, labels, search and permissions.
      </P>

      <P>
        That split is deliberate. Providers are good at getting mail through and bad at being a
        database you can query. Treating a provider response as the truth about your mailbox is how
        you end up unable to answer “what did we send this customer last March”.
      </P>

      <H2 id="the-model">A user is not an email account</H2>

      <P>
        This is the one idea the rest of the product follows from, so it is worth a minute before
        anything else.
      </P>

      <CodeBlock copyable={false}>{`User            you, signed in
 └─ MailAccount    one provider connection, with its own API key
     └─ Domain       company.com
         └─ MailAddress  support@company.com`}</CodeBlock>

      <P>
        A <Code>User</Code> is a person. A <Code>MailAccount</Code> is a connection to a provider,
        carrying its own credentials and owning its own mailbox. Domains hang off an account, and
        sender addresses hang off a domain.
      </P>

      <P>Several things follow directly:</P>

      <UL>
        <li>
          Threads, messages, drafts and labels belong to a <em>mail account</em>, never to you
          directly. The unified inbox is a query across the accounts you own, not a shared pile.
        </li>
        <li>
          Adding a second account never disturbs the first. Its credentials, its domains and its
          mail are separate all the way down to the database rows.
        </li>
        <li>
          An API token can be limited to one mail account, so an integration that only needs{' '}
          <Code>support@</Code> is not also trusted with your personal mail.
        </li>
        <li>
          Removing an account removes its mail with it, and leaves your other accounts untouched.
        </li>
      </UL>

      <Callout tone="note" title="Why this matters when reading the rest">
        <P>
          Almost every endpoint and every screen takes a mail account, explicitly or implicitly.
          When something asks “which account?”, it is asking which mailbox — not which login.
        </P>
      </Callout>

      <H2 id="pieces">What it is made of</H2>

      <UL>
        <li>
          <strong>API</strong> — a Go service. Everything the interface can do, this can do.
        </li>
        <li>
          <strong>Worker</strong> — the same binary in a different mode, running the jobs that
          must not happen inside a request: sending, and parsing inbound mail.
        </li>
        <li>
          <strong>PostgreSQL</strong> — the mailbox. All of it.
        </li>
        <li>
          <strong>Redis</strong> — the job queue, rate-limit counters, and the fan-out that gets a
          change from the worker to your open browser tabs.
        </li>
        <li>
          <strong>Web</strong> — the interface you are reading this in, served as static files.
        </li>
        <li>
          <strong>Object storage</strong> — attachments. The local filesystem in development,
          S3-compatible storage in production. Attachment bytes never go in the database.
        </li>
      </UL>

      <H2 id="how-mail-moves">How mail actually moves</H2>

      <P>
        <strong>Outbound.</strong> You write a draft. Sending it validates the draft, turns it into
        a message, and queues a job — then answers you. A worker picks the job up, decrypts that
        account's provider key and makes the call. This is why a sent message appears immediately
        with a “Sending” badge rather than after a pause: the provider has not been called yet.
      </P>

      <P>
        <strong>Inbound.</strong> Your provider posts to a webhook URL that is unique to one mail
        account. x-mail checks the signature, records the event and returns straight away. A worker
        then does the real work: parsing the message, sanitising its HTML, working out which thread
        it belongs to from its headers, and storing its attachments.
      </P>

      <P>
        Both paths are built so a retry is safe. A provider that delivers the same webhook twice
        produces one message, and a worker that dies mid-send does not send twice on restart.
      </P>

      <H2 id="where-next">Where to go next</H2>

      <UL>
        <li>
          <Link to="/docs" className="font-medium underline underline-offset-4">
            This guide
          </Link>{' '}
          continues with installing x-mail, then using it, then calling it.
        </li>
        <li>
          If you just want it running, the installation guide is the shortest path.
        </li>
      </UL>
    </>
  )
}
