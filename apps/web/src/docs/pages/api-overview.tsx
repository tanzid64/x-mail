import { Callout, Cell, Code, CodeBlock, H2, H3, Lede, P, Row, Table, UL } from '@/components/docs/prose'

export function ApiOverviewPage() {
  return (
    <>
      <Lede>
        Everything the interface does, the API does — because the interface is built on it. There
        is no private back channel and no endpoint reserved for the first-party client.
      </Lede>

      <H2 id="base-url">Base URL</H2>

      <P>
        Every endpoint sits under <Code>/api/v1</Code> on the host you deployed to. The paths in
        this reference are written relative to that.
      </P>

      <CodeBlock>{`https://your-host/api/v1`}</CodeBlock>

      <H2 id="auth">Authenticating</H2>

      <P>Two credentials work everywhere, and you choose per request.</P>

      <H3 id="auth-token">API token</H3>

      <P>
        What a script or a service should use. Create one under{' '}
        <strong>Settings → API tokens</strong> and send it as a bearer token.
      </P>

      <CodeBlock>{`curl https://your-host/api/v1/me \\
  -H "Authorization: Bearer xm_7Kd9..."`}</CodeBlock>

      <H3 id="auth-session">Session cookie</H3>

      <P>
        What the browser uses. <Code>POST /auth/login</Code> sets an httpOnly cookie that is sent
        automatically afterwards. Useful if you are building something that runs in a browser as a
        signed-in user; wrong for anything running on a server.
      </P>

      <Callout tone="note" title="Two endpoints refuse tokens">
        <P>
          Managing tokens and changing your password both require a session and answer{' '}
          <Code>403</Code> to a token. A token that could mint another would make expiry
          meaningless, and one that could change the password could lock out the person trying to
          revoke it.
        </P>
      </Callout>

      <H2 id="scoping">Account scope</H2>

      <P>
        Most endpoints take an optional <Code>mail_account_id</Code>. Supply it to work within one
        mailbox; leave it out and the request covers every account you own.
      </P>

      <P>
        A token limited to a single account is narrowed regardless of what you ask for. Listing
        accounts returns just the one, and naming any other answers <Code>404</Code>.
      </P>

      <H2 id="content-type">Requests</H2>

      <P>
        Send <Code>Content-Type: application/json</Code> with a JSON body. The one exception is
        uploading an attachment, which is <Code>multipart/form-data</Code>.
      </P>

      <P>
        On <Code>PATCH</Code>, omitting a field leaves it unchanged. That is different from sending{' '}
        <Code>null</Code>, which clears a nullable field.
      </P>

      <H2 id="responses">Responses</H2>

      <P>
        Successful responses wrap their payload in a key named after the thing returned, rather
        than returning a bare object. It costs one level of nesting and means a response can gain a
        sibling field later without breaking anyone.
      </P>

      <CodeBlock title="GET /mail-accounts/{id}">{`{
  "mail_account": {
    "id": "8f14e45f-ceea-467a-9b8f-6f2d3c4a5b6c",
    "name": "Support",
    "provider": "resend",
    "api_key_hint": "re_123…wxyz",
    "status": "active",
    "is_default": true
  }
}`}</CodeBlock>

      <P>
        Lists use the plural: <Code>mail_accounts</Code>, <Code>threads</Code>, <Code>drafts</Code>
        . A deletion returns <Code>204</Code> with no body at all.
      </P>

      <H2 id="errors">Errors</H2>

      <P>Every failure uses one shape, whatever went wrong.</P>

      <CodeBlock>{`{
  "error": {
    "code": "validation_failed",
    "message": "Please correct the highlighted fields.",
    "fields": {
      "email": "Enter a valid email address."
    }
  }
}`}</CodeBlock>

      <UL>
        <li>
          <Code>code</Code> is stable and machine-readable. Branch on this.
        </li>
        <li>
          <Code>message</Code> is safe to show a person. It never contains internal detail.
        </li>
        <li>
          <Code>fields</Code> appears only for validation failures, keyed by the field names of the
          request you sent.
        </li>
      </UL>

      <Table head={['Status', 'Code', 'Means']}>
        <Row>
          <Cell mono>400</Cell>
          <Cell mono>bad_request</Cell>
          <Cell>Malformed — bad JSON, an unparseable id, an unknown enum value.</Cell>
        </Row>
        <Row>
          <Cell mono>401</Cell>
          <Cell mono>unauthorized</Cell>
          <Cell>
            No valid credential. For a token the message says which of invalid, expired or revoked.
          </Cell>
        </Row>
        <Row>
          <Cell mono>403</Cell>
          <Cell mono>forbidden</Cell>
          <Cell>Authenticated, but this credential cannot do that. Rare by design — see below.</Cell>
        </Row>
        <Row>
          <Cell mono>404</Cell>
          <Cell mono>not_found</Cell>
          <Cell>No such record, or it is not yours. Deliberately the same answer.</Cell>
        </Row>
        <Row>
          <Cell mono>409</Cell>
          <Cell mono>conflict</Cell>
          <Cell>A name already taken, an address in use, a draft already sending.</Cell>
        </Row>
        <Row>
          <Cell mono>422</Cell>
          <Cell mono>validation_failed</Cell>
          <Cell>
            Well-formed but rejected. Check <Code>fields</Code>.
          </Cell>
        </Row>
        <Row>
          <Cell mono>429</Cell>
          <Cell mono>rate_limited</Cell>
          <Cell>Too many requests. Back off.</Cell>
        </Row>
        <Row>
          <Cell mono>500</Cell>
          <Cell mono>internal_error</Cell>
          <Cell>Our fault. The detail is in the server log, not in the response.</Cell>
        </Row>
      </Table>

      <Callout tone="note" title="Why you will see 404 where you expected 403">
        <P>
          Anything belonging to another user — or outside a limited token's account — answers{' '}
          <Code>404</Code>. A <Code>403</Code> would confirm the record exists, which is itself a
          disclosure. The exceptions are the two endpoints that genuinely refuse tokens.
        </P>
      </Callout>

      <H2 id="ids">Identifiers</H2>

      <P>
        Every id is a UUID. An id that is not a valid UUID answers <Code>404</Code> rather than{' '}
        <Code>400</Code> — from the caller's point of view a malformed id and a nonexistent one
        amount to the same thing.
      </P>

      <H2 id="pagination">Pagination</H2>

      <P>
        Lists of conversations page by cursor. Pass <Code>next_cursor</Code> from one response as{' '}
        <Code>cursor</Code> on the next; an empty <Code>next_cursor</Code> means you have reached
        the end.
      </P>

      <CodeBlock>{`GET /threads?folder=inbox&limit=50
  → { "threads": [...], "next_cursor": "eyJsYXN0Ijoi..." }

GET /threads?folder=inbox&limit=50&cursor=eyJsYXN0Ijoi...
  → { "threads": [...], "next_cursor": "" }`}</CodeBlock>

      <P>
        There is no page number, and that is on purpose. A mailbox changes while you are reading
        it; an offset silently skips or repeats conversations as new mail arrives above them.
      </P>

      <H2 id="rate-limits">Rate limits</H2>

      <P>
        Limits apply to the endpoints an attacker can reach without credentials, and to creating
        tokens.
      </P>

      <Table head={['Endpoint', 'Limit']}>
        <Row>
          <Cell mono>POST /auth/login</Cell>
          <Cell>10 per minute, per IP</Cell>
        </Row>
        <Row>
          <Cell mono>POST /auth/register</Cell>
          <Cell>5 per hour, per IP</Cell>
        </Row>
        <Row>
          <Cell mono>POST /auth/forgot-password</Cell>
          <Cell>5 per hour, per IP</Cell>
        </Row>
        <Row>
          <Cell mono>POST /auth/reset-password</Cell>
          <Cell>5 per hour, per IP</Cell>
        </Row>
        <Row>
          <Cell mono>POST /api-tokens</Cell>
          <Cell>20 per hour, per IP</Cell>
        </Row>
      </Table>

      <P>
        Ordinary mailbox endpoints are not rate limited per token today. Do not read that as
        permission to hammer them.
      </P>

      <H2 id="async">Some things are not finished when they answer</H2>

      <P>
        Sending answers <Code>202</Code>, not <Code>200</Code>. The message exists and a job is
        queued; the provider has not been called yet. Poll the message's <Code>status</Code>, or
        listen on the websocket, to follow it.
      </P>

      <P>
        Inbound webhooks behave the same way: the endpoint records the event and returns, and the
        message appears once a worker has parsed it.
      </P>

      <H2 id="curl">A complete example</H2>

      <P>Connect nothing, assume an account and a sender address already exist, and send a message.</P>

      <CodeBlock>{`TOKEN="xm_7Kd9..."
HOST="https://your-host/api/v1"

# 1. Find the account and a sender address on it.
ACCOUNT=$(curl -s "$HOST/mail-accounts" \\
  -H "Authorization: Bearer $TOKEN" | jq -r '.mail_accounts[0].id')

SENDER=$(curl -s "$HOST/addresses?mail_account_id=$ACCOUNT" \\
  -H "Authorization: Bearer $TOKEN" | jq -r '.addresses[0].id')

# 2. Create the draft.
DRAFT=$(curl -s -X POST "$HOST/drafts" \\
  -H "Authorization: Bearer $TOKEN" \\
  -H "Content-Type: application/json" \\
  -d "{
    \\"mail_account_id\\": \\"$ACCOUNT\\",
    \\"from_address_id\\": \\"$SENDER\\",
    \\"to\\": [{ \\"email\\": \\"customer@example.com\\" }],
    \\"subject\\": \\"Your order shipped\\",
    \\"html_body\\": \\"<p>On its way.</p>\\"
  }" | jq -r '.draft.id')

# 3. Send it. This answers 202 — the worker takes it from here.
curl -s -X POST "$HOST/drafts/$DRAFT/send" \\
  -H "Authorization: Bearer $TOKEN"`}</CodeBlock>
    </>
  )
}
