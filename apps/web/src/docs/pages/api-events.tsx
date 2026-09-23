import { Endpoint, Field, Fields, Part } from '@/components/docs/endpoint'
import { Callout, Cell, Code, CodeBlock, H2, Lede, P, Row, Table, UL } from '@/components/docs/prose'

export function ApiEventsPage() {
  return (
    <>
      <Lede>
        Two channels carry change: a websocket that tells your client something moved, and a
        webhook your provider posts to when mail arrives or a delivery attempt resolves.
      </Lede>

      <H2 id="websocket">Websocket</H2>

      <Endpoint id="ws" method="GET" path="/ws">
        <P>
          Upgrades to a websocket carrying only your own events. Authenticated like any other
          request — a session cookie, or a bearer token if your client can set headers on the
          upgrade.
        </P>
        <Part title="Returns">
          <P>
            <Code>101 Switching Protocols</Code>, or <Code>401</Code>.
          </P>
        </Part>
        <Part title="Messages">
          <CodeBlock>{`{
  "type": "email.received",
  "mail_account_id": "8f14e45f-ceea-467a-9b8f-6f2d3c4a5b6c",
  "data": { "thread_id": "7c9e6679-…", "message_id": "16fd2706-…" },
  "at": "2026-01-12T09:30:01Z"
}`}</CodeBlock>
          <Table head={['type', 'Sent when']}>
            <Row>
              <Cell mono>email.received</Cell>
              <Cell>Inbound mail has been parsed and stored.</Cell>
            </Row>
            <Row>
              <Cell mono>email.updated</Cell>
              <Cell>A message changed — usually a delivery status moving forward.</Cell>
            </Row>
            <Row>
              <Cell mono>thread.updated</Cell>
              <Cell>A conversation was read, starred, moved or labelled.</Cell>
            </Row>
            <Row>
              <Cell mono>draft.updated</Cell>
              <Cell>A draft was saved or sent, possibly in another tab.</Cell>
            </Row>
          </Table>
        </Part>
        <Part title="Notes">
          <Callout tone="security" title="Events carry ids, never content">
            <P>
              There is no message body on this channel, by design. Treat an event as “this changed,
              ask again” and refetch through the normal endpoints, which apply the same
              authorization they always do. A bug in the fan-out then cannot leak mail — the worst
              it can do is tell you to refetch something you are allowed to read anyway.
            </P>
          </Callout>
          <UL>
            <li>
              <strong>Reconnect on close.</strong> Drops are ordinary. Refetch on reconnect, since
              you will have missed whatever happened while you were away.
            </li>
            <li>
              <strong>Keep reading.</strong> A client that stops consuming is disconnected rather
              than waited on — blocking on one stalled reader would hold up everyone else.
            </li>
            <li>
              <strong>Origins are checked</strong> against <Code>CORS_ORIGINS</Code>, so a browser
              client must be served from a configured origin.
            </li>
          </UL>
        </Part>
      </Endpoint>

      <H2 id="webhooks">Provider webhook</H2>

      <Callout tone="note" title="You do not call this">
        <P>
          It is where your provider delivers inbound mail and delivery events. It is documented so
          you know what it expects and what it guarantees, not so you can invoke it.
        </P>
      </Callout>

      <Endpoint id="webhook" method="POST" path="/webhooks/resend/{mailAccountID}">
        <P>
          Receives Resend's callbacks for one mail account. Outside the session middleware — a
          provider has no cookie, and the signature is what authenticates the request.
        </P>
        <Part title="Headers">
          <Fields>
            <Field name="svix-id" type="string" required>
              Event identifier. Also the deduplication key.
            </Field>
            <Field name="svix-timestamp" type="string" required>
              Covered by the signature, so an old request cannot be replayed.
            </Field>
            <Field name="svix-signature" type="string" required>
              HMAC-SHA256 over <Code>{'{id}.{timestamp}.{body}'}</Code>, keyed by that account's
              signing secret.
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <P>
            <Code>200</Code> once recorded, or if it was already seen. <Code>401</Code> if the
            signature does not verify, <Code>404</Code> for an unknown account — the same answer a
            valid-but-unauthenticated caller gets, so this cannot be used to discover which
            accounts exist.
          </P>
        </Part>
        <Part title="What it guarantees">
          <UL>
            <li>
              <strong>It returns quickly.</strong> Verify, record, respond. Parsing, threading,
              sanitising and attachments all happen in a background job — a webhook that does real
              work eventually times out on a large message, gets retried, and turns one email into
              several.
            </li>
            <li>
              <strong>Duplicates collapse.</strong> Events are deduplicated on the provider's own
              event id, enforced by a uniqueness constraint rather than a check-then-insert. Two
              deliveries can race a check; they cannot race a constraint. The same callback twice
              produces exactly one message.
            </li>
            <li>
              <strong>Status only moves forward.</strong> Providers reorder events. A “sent”
              arriving after a “delivered” is ignored rather than allowed to walk the message
              backwards.
            </li>
          </UL>
        </Part>
        <Part title="Setting it up">
          <P>
            The URL is per account so there is exactly one secret to verify a given callback
            against, rather than every secret in the installation. Get it from the account's page
            in settings, or build it yourself, then store the signing secret with{' '}
            <Code>POST /mail-accounts/{'{id}'}/webhook-secret</Code>.
          </P>
          <CodeBlock>{`https://your-host/api/v1/webhooks/resend/<mail_account_id>`}</CodeBlock>
        </Part>
      </Endpoint>

      <H2 id="health">Health</H2>

      <P>
        None of these need a credential. The two probes sit outside <Code>/api/v1</Code> and answer{' '}
        <Code>GET</Code> or <Code>HEAD</Code>, since orchestrators use both.
      </P>

      <Table head={['Endpoint', 'Answers 200 when']}>
        <Row>
          <Cell mono>/api/v1/ping</Cell>
          <Cell>
            The API is routable. Answers <Code>{'{ "message": "pong" }'}</Code> and checks nothing
            else — useful for confirming a proxy is wired up.
          </Cell>
        </Row>
        <Row>
          <Cell mono>/healthz</Cell>
          <Cell>The process is up. Does not touch the database.</Cell>
        </Row>
        <Row>
          <Cell mono>/readyz</Cell>
          <Cell>
            PostgreSQL and Redis are reachable too. This is the one to put behind a load balancer.
          </Cell>
        </Row>
      </Table>

      <P>
        The worker exposes the same two on its own port, so it can be orchestrated like anything
        else.
      </P>
    </>
  )
}
