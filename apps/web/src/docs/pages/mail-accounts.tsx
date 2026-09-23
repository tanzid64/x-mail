import { Callout, Cell, Code, H2, H3, Lede, P, Row, Table, UL } from '@/components/docs/prose'

export function MailAccountsPage() {
  return (
    <>
      <Lede>
        A mail account is one connection to a provider. It carries its own API key, its own
        domains, its own sender addresses and its own mail. Everything else in x-mail hangs off
        one.
      </Lede>

      <P>
        You will find them all under <strong>Settings → Mail accounts</strong>.
      </P>

      <H2 id="connecting">Connecting one</H2>

      <P>
        <strong>Add mail account</strong> asks for three things: a name, a colour and a provider
        API key.
      </P>

      <UL>
        <li>
          The <strong>name</strong> is only ever for you. It is what the account switcher shows, so
          “Support” beats “Resend 2”.
        </li>
        <li>
          The <strong>colour</strong> is the dot next to it in the switcher, and the tint on rows
          in the unified inbox. With more than two accounts it becomes the fastest way to tell at a
          glance where a message came from.
        </li>
        <li>
          The <strong>API key</strong> is checked against the provider before anything is stored.
        </li>
      </UL>

      <P>
        That last point matters more than it sounds. A key that is wrong, revoked or from the
        wrong account is rejected on the form, immediately — not silently accepted and then
        discovered at your first send days later.
      </P>

      <Callout tone="security" title="The key never comes back">
        <P>
          Once accepted it is encrypted with a master key that lives outside the database, and only
          ever decrypted inside the process that is about to call the provider. The interface shows
          you a masked hint such as <Code>re_123…wxyz</Code> and nothing more. There is no screen,
          anywhere, that will show you the key again.
        </P>
      </Callout>

      <P>
        Connecting also pulls in the account's domains straight away, so you usually have something
        to work with before you have finished reading the page.
      </P>

      <H2 id="managing">Looking after an account</H2>

      <P>Opening an account gives you everything about it on one page.</P>

      <H3 id="status">Status</H3>

      <P>
        A badge next to the name says whether the provider still accepts the stored key.{' '}
        <strong>Test key</strong> re-checks on demand and tells you which of three things happened:
      </P>

      <Table head={['Result', 'Meaning']}>
        <Row>
          <Cell>The API key works</Cell>
          <Cell>The provider accepted it. Nothing to do.</Cell>
        </Row>
        <Row>
          <Cell>The provider rejected the stored API key</Cell>
          <Cell>
            It has been revoked or rotated at the provider. Paste a new one below; sending will
            fail until you do.
          </Cell>
        </Row>
        <Row>
          <Cell>Could not reach the provider</Cell>
          <Cell>
            A network problem, not a credentials problem. The key is probably fine — try again.
          </Cell>
        </Row>
      </Table>

      <P>
        The distinction is worth having: the middle case needs you to do something, and the last
        one usually needs you to wait.
      </P>

      <H3 id="rotating">Replacing the key</H3>

      <P>
        Paste a new key and choose <strong>Update key</strong>. It is verified before it replaces
        the old one, so a typo cannot lock the account out, and a successful replacement clears any
        recorded credential failure.
      </P>

      <H3 id="default">The default account</H3>

      <P>
        One account is always the default. It is the one compose starts on when you have not picked
        anything, and the first account you connect becomes it automatically. Use{' '}
        <strong>Make default</strong> to move it.
      </P>

      <H3 id="removing">Removing an account</H3>

      <Callout tone="warning" title="This takes the mail with it">
        <P>
          Removing an account deletes its domains, its addresses and every message stored under it.
          Your other accounts are untouched, and if you removed the default, the oldest remaining
          account takes over so you are never left without one. It cannot be undone.
        </P>
      </Callout>

      <H2 id="domains">Domains</H2>

      <P>
        Domains come from your provider — x-mail does not create them. You add a domain at Resend,
        then pull it in here.
      </P>

      <UL>
        <li>
          <strong>Sync from provider</strong> reconciles the whole list: new domains appear, and
          ones you removed at the provider disappear here too.
        </li>
        <li>
          <strong>Recheck DNS</strong> re-reads a single domain and leaves the others alone. This
          is the one to use while waiting for records to propagate.
        </li>
      </UL>

      <P>A domain is in one of three states:</P>

      <Table head={['Badge', 'What it means']}>
        <Row>
          <Cell>Verified</Cell>
          <Cell>Ready to send from. You can add addresses on it.</Cell>
        </Row>
        <Row>
          <Cell>Pending DNS</Cell>
          <Cell>
            The provider is waiting on records. They are listed on the card — publish them at your
            DNS host, then recheck.
          </Cell>
        </Row>
        <Row>
          <Cell>Failed</Cell>
          <Cell>Verification was attempted and did not pass. Check the records match exactly.</Cell>
        </Row>
      </Table>

      <Callout tone="tip" title="DNS takes its time">
        <P>
          Propagation is usually minutes but can be hours. A domain sitting at “Pending DNS” after
          a recheck is normal; x-mail tells you it is still pending rather than pretending
          something went wrong.
        </P>
      </Callout>

      <H2 id="addresses">Sender addresses</H2>

      <P>
        An address is an identity you can send from. Add one from its domain's card, with the
        address itself and a display name — the name recipients see next to it.
      </P>

      <P>Two rules are enforced:</P>

      <UL>
        <li>
          <strong>The address must be on that domain.</strong>{' '}
          <Code>hello@elsewhere.com</Code> on <Code>company.com</Code> is refused, because the
          provider would refuse to send it anyway.
        </li>
        <li>
          <strong>Addresses are unique across all of x-mail.</strong> Inbound routing resolves a
          recipient by address alone, so two accounts claiming one would make delivery ambiguous.
        </li>
      </UL>

      <P>
        The first address on a domain becomes its default sender. Add as many as you like —{' '}
        <Code>support@</Code>, <Code>billing@</Code>, <Code>noreply@</Code> — and each appears in
        the compose window's sender list.
      </P>

      <H2 id="switching">Switching between accounts</H2>

      <P>
        The switcher sits at the top of the sidebar. Pick one account to see only its mail, or{' '}
        <strong>All accounts</strong> to read every mailbox at once.
      </P>

      <P>
        Your choice is remembered, so reopening x-mail returns you to the same view. It is a filter
        and nothing more — switching never signs you in or out of anything, and the server checks
        what you are allowed to see on every request regardless of what the switcher says.
      </P>
    </>
  )
}
