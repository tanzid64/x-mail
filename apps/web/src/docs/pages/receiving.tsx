import { Callout, Code, H2, Lede, P, Step, Steps, UL } from '@/components/docs/prose'

export function ReceivingPage() {
  return (
    <>
      <Lede>
        Sending works as soon as you have a verified domain. Receiving needs one more step,
        because your provider has to be told where to put inbound mail.
      </Lede>

      <H2 id="setup">Setting it up</H2>

      <Steps>
        <Step title="Copy the webhook URL">
          <P>
            Open the mail account and find <strong>Receiving mail</strong>. The URL there ends in
            that account's identifier — it is specific to one account, not to your whole
            installation.
          </P>
        </Step>

        <Step title="Add it as a webhook at your provider">
          <P>
            In Resend, create a webhook pointing at that URL and subscribe it to the inbound and
            delivery events you care about.
          </P>
        </Step>

        <Step title="Paste the signing secret back">
          <P>
            Resend gives you a signing secret when you create the webhook. Paste it into the same
            section and save.
          </P>
        </Step>
      </Steps>

      <Callout tone="security" title="Without the secret, nothing arrives">
        <P>
          A callback whose signature does not verify is rejected. That is the point: the URL is
          public and unauthenticated, so the signature is the only thing distinguishing your
          provider from anyone who guessed it. An unsigned endpoint would let a stranger inject
          mail into your mailbox.
        </P>
      </Callout>

      <P>
        The URL is per account so that x-mail has exactly one secret to check a given callback
        against, rather than trying every secret in the system. A deployment-wide fallback secret
        exists for single-tenant installs, but the per-account one is better.
      </P>

      <H2 id="what-happens">What happens when mail arrives</H2>

      <P>
        The webhook endpoint does as little as possible: it verifies the signature, records the
        event and returns. Everything real happens in a background job.
      </P>

      <UL>
        <li>
          <strong>Routing.</strong> The recipient address decides which account the message belongs
          to. This is why addresses are unique across the whole installation — two accounts
          claiming one would make it ambiguous.
        </li>
        <li>
          <strong>Threading.</strong> The message's own headers come first —{' '}
          <Code>Message-ID</Code>, <Code>In-Reply-To</Code>, <Code>References</Code> — with subject
          and participant matching only as a fallback. Headers are what the sending client actually
          meant.
        </li>
        <li>
          <strong>Sanitising.</strong> The HTML body is cleaned once, on arrival, and stored
          alongside the untouched original. The cost is paid per message rather than per view, and
          keeping the original means a future sanitiser fix can be re-applied instead of the
          message being lost.
        </li>
        <li>
          <strong>Attachments.</strong> Pulled out and written to object storage, with the message
          keeping only a reference.
        </li>
      </UL>

      <P>
        Doing that work inside the webhook request would eventually time out on a large message,
        the provider would retry, and one email would become several.
      </P>

      <H2 id="duplicates">Duplicate callbacks</H2>

      <P>
        Providers redeliver by design — a timeout on their side means they try again, whether or
        not you actually received it. Events are deduplicated on the provider's own event
        identifier, enforced by a uniqueness constraint in the database rather than by checking
        first and inserting after. Two deliveries can race a check; they cannot race a constraint.
      </P>

      <P>The same callback arriving twice produces exactly one message.</P>

      <H2 id="delivery-events">Delivery events</H2>

      <P>
        The same webhook carries what happened to mail you sent: accepted, delivered, bounced,
        complained. These update the badge on the message.
      </P>

      <P>
        Because providers reorder events, status only ever moves forward. A “sent” arriving after a
        “delivered” is ignored rather than allowed to walk the message backwards.
      </P>

      <H2 id="troubleshooting">When mail does not arrive</H2>

      <UL>
        <li>
          <strong>Check the signing secret.</strong> A mismatch rejects every callback silently
          from your side — the provider will show them failing.
        </li>
        <li>
          <strong>Check the address exists in x-mail.</strong> Mail to an address you have not
          added has nowhere to be routed.
        </li>
        <li>
          <strong>Check the worker is running.</strong> Events are recorded by the API but turned
          into messages by the worker. Without it, callbacks succeed and nothing appears.
        </li>
        <li>
          <strong>Check the URL is reachable.</strong> It has to be public — your provider cannot
          post to <Code>localhost</Code>. A tunnel is the usual answer while developing.
        </li>
      </UL>
    </>
  )
}
