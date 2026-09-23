import { Callout, Code, H2, Lede, P, Step, Steps, UL } from '@/components/docs/prose'

export function GettingStartedPage() {
  return (
    <>
      <Lede>
        x-mail is installed and you are looking at an empty inbox. Four steps turn that into a
        working mailbox you can send and receive from.
      </Lede>

      <P>
        You will need a Resend account and an API key from it. x-mail does not deliver mail itself
        — it needs a provider to hand messages to.
      </P>

      <Steps>
        <Step title="Create your user account">
          <P>
            Sign up with your email and a password of at least twelve characters. The first person
            to register is just a normal user; there is no seeded admin and no default password.
          </P>
          <P>
            A confirmation email is sent. Until you click it a banner stays at the top of the app —
            you can use everything in the meantime.
          </P>
        </Step>

        <Step title="Connect a provider account">
          <P>
            Go to <strong>Settings → Mail accounts</strong> and choose <strong>Add mail account</strong>.
            Give it a name you will recognise in the account switcher — “Support”, “Personal” — and
            paste your Resend API key.
          </P>
          <P>
            The key is checked against Resend before anything is saved, so a wrong one is reported
            on the field right there rather than at your first send days later. Once accepted it is
            encrypted, and the interface only ever shows you a masked hint of it again.
          </P>
        </Step>

        <Step title="Add a sender address">
          <P>
            Connecting an account pulls in its domains automatically. Open the account and you will
            see them, each marked verified or pending.
          </P>
          <UL>
            <li>
              A <strong>verified</strong> domain is ready. Add an address on it — say{' '}
              <Code>support@company.com</Code> — and give it a display name.
            </li>
            <li>
              A <strong>pending</strong> domain lists the DNS records it still needs. Publish them
              at your DNS host, then use <strong>Recheck DNS</strong>. Propagation can take a
              while; the badge tells you where it has got to.
            </li>
          </UL>
        </Step>

        <Step title="Send something">
          <P>
            Hit <strong>Compose</strong>, pick the address you just added as the sender, and send
            yourself a message.
          </P>
          <P>
            It appears in <strong>Sent</strong> straight away with a “Sending” badge, because
            sending is asynchronous: x-mail has queued it and a background worker is making the
            provider call. The badge moves to “Sent”, then “Delivered”, on its own.
          </P>
        </Step>
      </Steps>

      <Callout tone="warning" title="Stuck on “Sending”?">
        <P>
          The worker is not running. The API queues, the worker delivers — with Docker it starts
          alongside everything else, but running locally it needs its own <Code>make worker</Code>.
        </P>
      </Callout>

      <H2 id="receiving">Receiving mail too</H2>

      <P>
        Sending works now. Receiving needs one more step, because Resend has to be told where to
        deliver inbound mail.
      </P>

      <P>
        On the account page, <strong>Receiving mail</strong> shows a webhook URL unique to that
        account. Add it as a webhook in Resend, then paste the signing secret Resend gives you back
        into the same section. Mail that arrives without a valid signature is rejected, so this is
        not optional.
      </P>

      <H2 id="more-accounts">Adding the rest</H2>

      <P>
        That is the whole loop. A second account is the same three steps again, and once it exists
        the switcher at the top of the sidebar gains an entry — plus <strong>All accounts</strong>,
        which reads every mailbox at once.
      </P>

      <P>
        Nothing is shared between accounts. Separate credentials, separate domains, separate mail,
        all the way down.
      </P>

      <H2 id="next">Where next</H2>

      <UL>
        <li>The interface, screen by screen, is covered in the pages that follow.</li>
        <li>
          If you would rather drive x-mail from code, skip ahead to API tokens and the API
          reference — everything here is available there.
        </li>
      </UL>
    </>
  )
}
