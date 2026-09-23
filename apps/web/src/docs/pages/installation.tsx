import { Callout, Code, CodeBlock, H2, H3, Lede, P, Step, Steps, UL } from '@/components/docs/prose'

export function InstallationPage() {
  return (
    <>
      <Lede>
        x-mail runs as four processes — an API, a worker, PostgreSQL and Redis — plus a static
        frontend. The quickest way to all of it is Docker.
      </Lede>

      <H2 id="requirements">Before you start</H2>

      <UL>
        <li>
          <strong>Docker</strong> with Compose v2, if you want the container route. Nothing else is
          needed.
        </li>
        <li>
          <strong>Go 1.26</strong> and <strong>Node 22</strong>, if you would rather run the API
          and the frontend directly on your machine.
        </li>
        <li>
          A <strong>Resend account</strong> and an API key. x-mail does not deliver mail itself, so
          without a provider you can sign in and look around but not send anything.
        </li>
      </UL>

      <Callout tone="note" title="You do not need a provider key to install">
        <P>
          Installing, signing in and exploring the interface all work without one. Connect a
          provider when you are ready to send — it is the first thing the app asks for.
        </P>
      </Callout>

      <H2 id="docker">Running it with Docker</H2>

      <Steps>
        <Step title="Get the code">
          <CodeBlock>{`git clone <your-fork> x-mail
cd x-mail`}</CodeBlock>
        </Step>

        <Step title="Create your environment file">
          <P>
            Copy the example and generate the two secrets. Both are required and the API refuses to
            start without them.
          </P>
          <CodeBlock>{`cp .env.example .env

# A 32-byte key for signing session cookies.
openssl rand -base64 32

# A separate 32-byte key that encrypts stored provider credentials.
openssl rand -base64 32`}</CodeBlock>
          <P>
            Put the first in <Code>SESSION_SECRET</Code> and the second in{' '}
            <Code>ENCRYPTION_MASTER_KEY</Code>. Use two different values — they protect different
            things and have different blast radii if one leaks.
          </P>
        </Step>

        <Step title="Start everything">
          <CodeBlock>{`make docker-up`}</CodeBlock>
          <P>
            This builds the images, starts PostgreSQL, Redis, the API, the worker and the web
            server, and waits for each to report healthy. The API applies any pending database
            migrations as it boots, so there is no separate migration step.
          </P>
        </Step>

        <Step title="Open it">
          <P>
            The app is at <Code>http://localhost:8180</Code>. Create the first account from the
            sign-up page — there is no seeded admin user and no default password.
          </P>
        </Step>
      </Steps>

      <H3 id="docker-commands">Everyday commands</H3>

      <CodeBlock>{`make docker-up      # build if needed, then start the whole stack
make docker-down    # stop the app containers, leave Postgres and Redis running
make docker-logs    # follow the API, worker and web logs
make docker-ps      # what is running, and whether it is healthy`}</CodeBlock>

      <Callout tone="warning" title="Ports">
        <P>
          The stack deliberately avoids the usual defaults, because they are so often already
          taken: the app is on <Code>8180</Code>, the API on <Code>8190</Code>, PostgreSQL on{' '}
          <Code>5433</Code> and Redis on <Code>6380</Code>. Change them in{' '}
          <Code>docker-compose.yml</Code> if those collide too.
        </P>
      </Callout>

      <H2 id="local">Running it on your machine</H2>

      <P>
        Useful if you are changing the code, since both the API and the frontend reload on save.
        The databases still run in containers; everything else runs natively.
      </P>

      <Steps>
        <Step title="Start the databases only">
          <CodeBlock>{`make infra`}</CodeBlock>
          <P>
            PostgreSQL on <Code>5433</Code> and Redis on <Code>6380</Code>, and nothing else.
          </P>
        </Step>

        <Step title="Run the API">
          <CodeBlock>{`make api`}</CodeBlock>
          <P>
            Listens on <Code>:8090</Code> and applies pending migrations on start.
          </P>
        </Step>

        <Step title="Run the worker">
          <P>
            In another terminal. Without it, mail is accepted and queued but never actually sent,
            and inbound webhooks are recorded but never turned into messages.
          </P>
          <CodeBlock>{`make worker`}</CodeBlock>
        </Step>

        <Step title="Run the frontend">
          <CodeBlock>{`make web`}</CodeBlock>
          <P>
            Vite serves the app on <Code>http://localhost:5180</Code> and proxies <Code>/api</Code>{' '}
            to the API, so the session cookie stays same-origin exactly as it does in production.
          </P>
        </Step>
      </Steps>

      <Callout tone="tip" title="The worker is not optional">
        <P>
          Sending is asynchronous by design: the API validates and queues, the worker delivers. If
          messages sit at “Sending” forever, the worker is not running.
        </P>
      </Callout>

      <H2 id="database">Database migrations</H2>

      <P>
        Migrations are compiled into the binary and applied automatically at startup under an
        advisory lock, so starting the API and the worker at the same time is safe and a fresh
        container never runs against an old schema. You only need these when developing:
      </P>

      <CodeBlock>{`make migrate-status          # what has been applied
make migrate-up              # apply anything pending
make migrate-down            # roll back the most recent one
make migrate-new name=add_x  # create an up/down pair`}</CodeBlock>

      <H2 id="verify">Checking it works</H2>

      <P>Two endpoints answer without a session, and both accept GET or HEAD:</P>

      <CodeBlock>{`curl http://localhost:8190/healthz   # the process is up
curl http://localhost:8190/readyz    # PostgreSQL and Redis are reachable too`}</CodeBlock>

      <P>
        <Code>/readyz</Code> is the one to point a load balancer at: it fails while a dependency is
        unreachable, which is precisely when you want traffic sent elsewhere.
      </P>

      <H2 id="tests">Running the tests</H2>

      <CodeBlock>{`make test      # Go tests plus a frontend typecheck
make e2e       # browser tests against a real, isolated stack
make lint      # go vet and oxlint`}</CodeBlock>

      <Callout tone="note" title="Integration tests need the databases">
        <P>
          Tests that need PostgreSQL skip themselves when <Code>TEST_DATABASE_URL</Code> is unset,
          so the suite stays green on a machine with nothing running. That also means a green run
          proves nothing if the infrastructure is down — start <Code>make infra</Code> first, and
          use <Code>make test-api-v</Code> when you want to see what actually ran.
        </P>
      </Callout>
    </>
  )
}
