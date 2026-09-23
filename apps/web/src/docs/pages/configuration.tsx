import { Callout, Cell, Code, CodeBlock, H2, Lede, P, Row, Table, UL } from '@/components/docs/prose'

export function ConfigurationPage() {
  return (
    <>
      <Lede>
        Every setting is an environment variable. In development they come from <Code>.env</Code>;
        in production pass them in properly — the container images deliberately ship without one.
      </Lede>

      <P>
        Configuration is validated once at startup, and every problem is reported together rather
        than one per restart. A missing secret or a key of the wrong length stops the process
        immediately instead of failing later on the first send.
      </P>

      <H2 id="required">Required</H2>

      <P>Three values have no default. Without them the API will not start.</P>

      <Table head={['Variable', 'What it is']}>
        <Row>
          <Cell mono>DATABASE_URL</Cell>
          <Cell>
            PostgreSQL connection string. This is the mailbox — all of it.
          </Cell>
        </Row>
        <Row>
          <Cell mono>SESSION_SECRET</Cell>
          <Cell>
            Signs session cookies. Must decode to at least 32 bytes. Changing it signs everyone
            out.
          </Cell>
        </Row>
        <Row>
          <Cell mono>ENCRYPTION_MASTER_KEY</Cell>
          <Cell>
            Encrypts stored provider credentials with AES-256-GCM, so it must decode to{' '}
            <em>exactly</em> 32 bytes.
          </Cell>
        </Row>
      </Table>

      <CodeBlock>{`openssl rand -base64 32   # SESSION_SECRET
openssl rand -base64 32   # ENCRYPTION_MASTER_KEY — a different value`}</CodeBlock>

      <Callout tone="security" title="Losing the master key loses the credentials">
        <P>
          Provider API keys are encrypted with it and the key is never stored in PostgreSQL — that
          separation is the whole point, since a database dump on its own then decrypts to nothing.
          If you lose it, every connected account has to have its key entered again. Back it up
          somewhere other than the database, and load it from a secrets manager in production.
        </P>
      </Callout>

      <H2 id="core">Server</H2>

      <Table head={['Variable', 'Default', 'Notes']}>
        <Row>
          <Cell mono>APP_ENV</Cell>
          <Cell mono>development</Cell>
          <Cell>
            Set to <Code>production</Code> to turn on the stricter checks below.
          </Cell>
        </Row>
        <Row>
          <Cell mono>HTTP_ADDR</Cell>
          <Cell mono>:8090</Cell>
          <Cell>Where the API listens.</Cell>
        </Row>
        <Row>
          <Cell mono>WORKER_HTTP_ADDR</Cell>
          <Cell mono>:8092</Cell>
          <Cell>
            The worker's own health endpoints. A worker that cannot be probed cannot be
            orchestrated.
          </Cell>
        </Row>
        <Row>
          <Cell mono>APP_BASE_URL</Cell>
          <Cell mono>http://localhost:5180</Cell>
          <Cell>
            Used to build links in the mail x-mail sends about itself — verification and password
            resets. Get this wrong and those links point nowhere.
          </Cell>
        </Row>
        <Row>
          <Cell mono>CORS_ORIGINS</Cell>
          <Cell mono>http://localhost:5180</Cell>
          <Cell>
            Comma-separated origins allowed to call the API with credentials. Also the allow-list
            for websocket origins.
          </Cell>
        </Row>
      </Table>

      <H2 id="sessions">Sessions</H2>

      <Table head={['Variable', 'Default', 'Notes']}>
        <Row>
          <Cell mono>SESSION_COOKIE_NAME</Cell>
          <Cell mono>xmail_session</Cell>
          <Cell>Change it if it collides with something else on the domain.</Cell>
        </Row>
        <Row>
          <Cell mono>SESSION_TTL_HOURS</Cell>
          <Cell mono>720</Cell>
          <Cell>Thirty days.</Cell>
        </Row>
        <Row>
          <Cell mono>SESSION_COOKIE_SECURE</Cell>
          <Cell mono>false</Cell>
          <Cell>
            Must be <Code>true</Code> when <Code>APP_ENV=production</Code>; startup refuses
            otherwise.
          </Cell>
        </Row>
      </Table>

      <H2 id="redis">Redis</H2>

      <P>
        Redis carries the job queue, the rate-limit counters and the pub/sub fan-out that gets a
        change from the worker to your open browser tabs.
      </P>

      <Table head={['Variable', 'Default']}>
        <Row>
          <Cell mono>REDIS_ADDR</Cell>
          <Cell mono>localhost:6380</Cell>
        </Row>
        <Row>
          <Cell mono>REDIS_PASSWORD</Cell>
          <Cell mono>(empty)</Cell>
        </Row>
        <Row>
          <Cell mono>REDIS_DB</Cell>
          <Cell mono>0</Cell>
        </Row>
      </Table>

      <H2 id="storage">Attachment storage</H2>

      <P>
        Attachment bytes never go in PostgreSQL — the row holds a storage key and nothing else. Use
        the local filesystem in development and object storage in production.
      </P>

      <Table head={['Variable', 'Default', 'Notes']}>
        <Row>
          <Cell mono>STORAGE_DRIVER</Cell>
          <Cell mono>local</Cell>
          <Cell>
            <Code>local</Code> or <Code>s3</Code>. Anything else is rejected at startup.
          </Cell>
        </Row>
        <Row>
          <Cell mono>STORAGE_LOCAL_PATH</Cell>
          <Cell mono>./storage</Cell>
          <Cell>Used by the local driver.</Cell>
        </Row>
        <Row>
          <Cell mono>S3_ENDPOINT</Cell>
          <Cell mono>(empty)</Cell>
          <Cell>Set for S3-compatible services such as Cloudflare R2; leave empty for AWS.</Cell>
        </Row>
        <Row>
          <Cell mono>S3_REGION</Cell>
          <Cell mono>auto</Cell>
          <Cell />
        </Row>
        <Row>
          <Cell mono>S3_BUCKET</Cell>
          <Cell mono>(empty)</Cell>
          <Cell>
            Required when <Code>STORAGE_DRIVER=s3</Code>.
          </Cell>
        </Row>
        <Row>
          <Cell mono>S3_ACCESS_KEY_ID</Cell>
          <Cell mono>(empty)</Cell>
          <Cell>Required for s3.</Cell>
        </Row>
        <Row>
          <Cell mono>S3_SECRET_ACCESS_KEY</Cell>
          <Cell mono>(empty)</Cell>
          <Cell>Required for s3.</Cell>
        </Row>
      </Table>

      <Callout tone="warning" title="The API and the worker must share storage">
        <P>
          The API writes an attachment when you upload it; the worker reads the same bytes when it
          sends. With the local driver that means a shared volume — the Compose file already wires
          one up. Object storage makes the problem go away.
        </P>
      </Callout>

      <H2 id="system-mail">Mail x-mail sends about itself</H2>

      <P>
        Email verification and password resets have to go out before a user has connected any
        provider account, so they use their own credentials. These are entirely separate from the
        keys users add in the app.
      </P>

      <Table head={['Variable', 'Default', 'Notes']}>
        <Row>
          <Cell mono>SYSTEM_RESEND_API_KEY</Cell>
          <Cell mono>(empty)</Cell>
          <Cell>
            Leave empty and those emails are logged instead of sent, which is usually what you want
            in development.
          </Cell>
        </Row>
        <Row>
          <Cell mono>SYSTEM_MAIL_FROM</Cell>
          <Cell mono>x-mail &lt;noreply@example.com&gt;</Cell>
          <Cell>Must be an address on a domain verified with that key.</Cell>
        </Row>
        <Row>
          <Cell mono>RESEND_WEBHOOK_SECRET</Cell>
          <Cell mono>(empty)</Cell>
          <Cell>
            A deployment-wide fallback for verifying provider callbacks. Prefer the per-account
            secret set in the app.
          </Cell>
        </Row>
      </Table>

      <H2 id="testing">Testing and escape hatches</H2>

      <P>
        Three variables exist only so the test suite can run a stub provider and register many
        users from one address. Two of them are refused outright in production.
      </P>

      <Table head={['Variable', 'Default', 'Notes']}>
        <Row>
          <Cell mono>RATE_LIMIT_SCALE</Cell>
          <Cell mono>1</Cell>
          <Cell>
            Multiplies every rate-limit allowance. Must be <Code>1</Code> in production.
          </Cell>
        </Row>
        <Row>
          <Cell mono>RESEND_BASE_URL</Cell>
          <Cell mono>(empty)</Cell>
          <Cell>
            Redirects provider traffic elsewhere. Refused in production — setting it there would
            send real credentials to whatever host you named.
          </Cell>
        </Row>
        <Row>
          <Cell mono>TEST_DATABASE_URL</Cell>
          <Cell mono>(empty)</Cell>
          <Cell>
            Integration tests skip themselves when unset. Never points at your development
            database: the tests truncate it.
          </Cell>
        </Row>
      </Table>

      <H2 id="production">What changes in production</H2>

      <P>
        Setting <Code>APP_ENV=production</Code> turns on checks that fail the process at startup
        rather than letting it run unsafely:
      </P>

      <UL>
        <li>
          <Code>SESSION_COOKIE_SECURE</Code> must be <Code>true</Code>.
        </li>
        <li>
          <Code>RESEND_BASE_URL</Code> must not be set.
        </li>
        <li>
          <Code>RATE_LIMIT_SCALE</Code> must be exactly <Code>1</Code>.
        </li>
      </UL>

      <P>
        The images contain no <Code>.env</Code> at all — baking one in would put the encryption
        master key inside a layer anyone with the image could read.
      </P>
    </>
  )
}
