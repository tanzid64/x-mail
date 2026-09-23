import { Callout, Cell, Code, CodeBlock, H2, Lede, P, Row, Table, UL } from '@/components/docs/prose'

export function ApiTokensPage() {
  return (
    <>
      <Lede>
        An API token lets a script, a server or another service act as you, without a browser and
        without your password.
      </Lede>

      <P>
        Find them under <strong>Settings → API tokens</strong>.
      </P>

      <H2 id="creating">Creating one</H2>

      <P>
        <strong>Create token</strong> asks for three things.
      </P>

      <UL>
        <li>
          <strong>A name.</strong> Only for you — it is how you will recognise which one to revoke
          later. “CI deploy” is a better name than “token 2”.
        </li>
        <li>
          <strong>An expiry.</strong> One of four: 15 days, 30 days, 1 year, or never.
        </li>
        <li>
          <strong>Access.</strong> Every mail account you own, or one of them.
        </li>
      </UL>

      <Callout tone="security" title="The token is shown once">
        <P>
          Only a hash of it is stored, so there is no screen that can show it to you again — not
          even to an administrator. Copy it when it appears. If you lose it, revoke it and create
          another; that is cheaper than the alternative, which would be a system that can hand your
          credentials back to anyone who asks.
        </P>
      </Callout>

      <H2 id="expiry">Choosing an expiry</H2>

      <P>Four options, and no free-text duration. Pick by what the token is for.</P>

      <Table head={['Expiry', 'Suits']}>
        <Row>
          <Cell>15 days</Cell>
          <Cell>A migration, a one-off script, a trial integration, anything you are testing.</Cell>
        </Row>
        <Row>
          <Cell>30 days</Cell>
          <Cell>A month of access, then it stops working on its own.</Cell>
        </Row>
        <Row>
          <Cell>1 year</Cell>
          <Cell>An integration you expect to keep running. Renew deliberately.</Cell>
        </Row>
        <Row>
          <Cell>Never</Cell>
          <Cell>
            Something that must not break unattended. Only revoking will stop it, so store it
            properly.
          </Cell>
        </Row>
      </Table>

      <P>
        The set is fixed on purpose. An arbitrary duration invites tokens that quietly outlive the
        thing they were made for, because nobody ever actually chose how long they should last.
      </P>

      <H2 id="scope">Limiting a token to one mailbox</H2>

      <P>
        By default a token reaches every mail account you own, including ones you add later. Choose
        a single account instead and it can only see and act on that one.
      </P>

      <P>
        This is worth doing whenever it fits. An integration that only needs to send from{' '}
        <Code>support@</Code> should not also be able to read your personal mail if it is
        compromised.
      </P>

      <P>
        A limited token does not get a polite refusal when it reaches outside its account — it gets{' '}
        <Code>404</Code>, the same answer it would get for an account belonging to someone else. A
        refusal would confirm the account exists.
      </P>

      <H2 id="using">Using one</H2>

      <P>
        Send it as a bearer token. Every endpoint that works with a browser session works with a
        token instead.
      </P>

      <CodeBlock>{`curl https://your-host/api/v1/threads \\
  -H "Authorization: Bearer xm_7Kd9..."`}</CodeBlock>

      <Callout tone="warning" title="Two things a token cannot do">
        <P>
          It cannot create or revoke tokens, and it cannot change your password. Both require a
          signed-in browser and answer <Code>403</Code> to a token.
        </P>
        <P>
          A token that could mint another would make every expiry meaningless — a 15-day key could
          issue itself a permanent one. A token that could change the password could lock you out
          of the page where you would revoke it.
        </P>
      </Callout>

      <H2 id="managing">Keeping track</H2>

      <P>The list shows, for each token, what it can reach and when it was last used.</P>

      <UL>
        <li>
          A masked hint such as <Code>xm_7Kd9…qR4</Code>, enough to match against a token you hold.
        </li>
        <li>How long until it expires, or that it never will. Under a week is called out.</li>
        <li>Which account it is limited to, if any.</li>
        <li>
          When it was last used, or that it never has — the quickest way to spot one nothing needs
          any more.
        </li>
      </UL>

      <H2 id="revoking">Revoking</H2>

      <P>
        Revoking takes effect on the token's next request. There is no grace period and no undo,
        which is what you want from the button you reach for when a key has leaked.
      </P>

      <P>
        Anything still using it starts getting <Code>401</Code> with a message saying the token was
        revoked — not that it is invalid. An integrator chasing a sudden failure should not be sent
        hunting for a typo.
      </P>

      <H2 id="leaked">If a token leaks</H2>

      <UL>
        <li>
          <strong>Revoke it.</strong> First, before anything else. It is the only step that stops
          the bleeding.
        </li>
        <li>
          <strong>Create a replacement</strong> and deploy it wherever the old one was.
        </li>
        <li>
          <strong>Check the sent mail</strong> on the accounts it could reach.
        </li>
        <li>
          <strong>Consider whether it needed that much access,</strong> and limit the replacement
          to one account if it does not.
        </li>
      </UL>

      <P>
        Tokens are prefixed <Code>xm_</Code> so that secret scanners can recognise one committed to
        a repository by accident. That helps, but it is not a substitute for keeping them out of
        source control.
      </P>
    </>
  )
}
