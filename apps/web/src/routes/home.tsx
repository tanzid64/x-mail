import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  AtSign,
  KeyRound,
  Layers,
  Lock,
  Radio,
  Server,
  ShieldCheck,
  Terminal,
} from 'lucide-react'

import { Button } from '@/components/ui/button'

export function HomePage() {
  return (
    <main>
      <Hero />
      <ObjectModel />
      <Features />
      <ApiTeaser />
      <SelfHost />
      <Closing />
    </main>
  )
}

function Hero() {
  return (
    <section className="mx-auto w-full max-w-6xl px-4 py-20 sm:py-28">
      <div className="max-w-3xl">
        <p className="mb-4 inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs text-muted-foreground">
          <Layers className="size-3.5" />
          Multi-account by design
        </p>
        <h1 className="text-4xl font-semibold tracking-tight text-balance sm:text-5xl">
          One mailbox for every address you send from.
        </h1>
        <p className="mt-5 max-w-2xl text-lg text-muted-foreground text-pretty">
          x-mail is a mail platform you run yourself. Connect the provider accounts you already
          have, bring your own domains, and read and reply to all of them in one Gmail-shaped
          inbox — with an API that does everything the screen does.
        </p>

        <div className="mt-8 flex flex-wrap gap-3">
          <Button asChild size="lg" className="gap-2">
            <Link to="/register">
              Create an account
              <ArrowRight className="size-4" />
            </Link>
          </Button>
          <Button asChild size="lg" variant="outline">
            <Link to="/docs">Read the documentation</Link>
          </Button>
        </div>

        <p className="mt-4 text-sm text-muted-foreground">
          Or{' '}
          <Link
            to="/docs"
            className="font-medium text-foreground underline underline-offset-4"
          >
            run it on your own machine
          </Link>{' '}
          in about five minutes.
        </p>
      </div>
    </section>
  )
}

/**
 * The one idea that explains every other decision in the product, so it goes
 * near the top rather than buried in the docs.
 */
function ObjectModel() {
  return (
    <section className="border-y bg-muted/30">
      <div className="mx-auto w-full max-w-6xl px-4 py-16">
        <div className="grid gap-10 md:grid-cols-2 md:items-center">
          <div>
            <h2 className="text-2xl font-semibold tracking-tight">
              A user is not an email account
            </h2>
            <p className="mt-4 text-muted-foreground text-pretty">
              Most mail tools assume you are one address. x-mail assumes you are a person with
              several. You sign in once; underneath, each provider connection carries its own
              credentials, its own domains and its own mailbox.
            </p>
            <p className="mt-4 text-muted-foreground text-pretty">
              That is why you can read <code className="font-mono text-sm">support@</code> and{' '}
              <code className="font-mono text-sm">you@</code> side by side without them ever
              becoming the same pile of mail — and why an API token can be handed to an
              integration for one mailbox and no others.
            </p>
          </div>

          <div className="rounded-xl border bg-background p-6 font-mono text-sm shadow-sm">
            <div className="flex flex-col gap-3">
              <ModelRow label="User" detail="you, signed in" />
              <ModelArrow />
              <ModelRow label="MailAccount" detail="one provider connection" />
              <ModelArrow />
              <ModelRow label="Domain" detail="company.com" />
              <ModelArrow />
              <ModelRow label="MailAddress" detail="support@company.com" />
            </div>
            <p className="mt-5 border-t pt-4 font-sans text-xs text-muted-foreground">
              Threads, messages, drafts and labels all hang off a mail account — never off you
              directly.
            </p>
          </div>
        </div>
      </div>
    </section>
  )
}

function ModelRow({ label, detail }: { label: string; detail: string }) {
  return (
    <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
      <span className="rounded-md bg-muted px-2 py-1 font-medium">{label}</span>
      <span className="font-sans text-xs text-muted-foreground">{detail}</span>
    </div>
  )
}

function ModelArrow() {
  return <span className="pl-3 text-muted-foreground">↓</span>
}

const FEATURES = [
  {
    icon: Layers,
    title: 'Unified inbox, separate mailboxes',
    body: 'Read everything at once, or one account at a time. Switching is a filter, never a different login.',
  },
  {
    icon: AtSign,
    title: 'Your domains, your addresses',
    body: 'Sync domains from your provider, publish the DNS records it asks for, and add as many sender addresses as you need.',
  },
  {
    icon: Radio,
    title: 'Live, without refreshing',
    body: 'Mail arrives, delivery status changes and unread counts move on their own over a websocket.',
  },
  {
    icon: ShieldCheck,
    title: 'Untrusted mail, treated as such',
    body: 'Incoming HTML is sanitised on arrival and rendered in a sandboxed frame with scripts off. Attachments never render inline.',
  },
  {
    icon: Lock,
    title: 'Credentials never reach the browser',
    body: 'Provider API keys are encrypted with a key that lives outside the database. The UI only ever sees a masked hint.',
  },
  {
    icon: KeyRound,
    title: 'Everything is an API',
    body: 'Every screen is built on the same public API. Create a token and a script can do whatever you can do.',
  },
]

function Features() {
  return (
    <section className="mx-auto w-full max-w-6xl px-4 py-16">
      <h2 className="text-2xl font-semibold tracking-tight">What you get</h2>
      <div className="mt-8 grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
        {FEATURES.map((feature) => (
          <div key={feature.title} className="rounded-xl border p-5">
            <feature.icon className="size-5 text-muted-foreground" />
            <h3 className="mt-3 font-medium">{feature.title}</h3>
            <p className="mt-1.5 text-sm text-muted-foreground text-pretty">{feature.body}</p>
          </div>
        ))}
      </div>
    </section>
  )
}

function ApiTeaser() {
  return (
    <section className="border-y bg-muted/30">
      <div className="mx-auto w-full max-w-6xl px-4 py-16">
        <div className="grid gap-10 lg:grid-cols-2 lg:items-center">
          <div>
            <h2 className="flex items-center gap-2 text-2xl font-semibold tracking-tight">
              <Terminal className="size-5" />
              Built on its own API
            </h2>
            <p className="mt-4 text-muted-foreground text-pretty">
              There is no private back channel. The inbox you use in the browser calls exactly the
              endpoints documented for you, with the same authorization rules.
            </p>
            <p className="mt-4 text-muted-foreground text-pretty">
              Create a token in settings, give it 15 days or a year, optionally limit it to a
              single mailbox, and hand it to whatever needs to send.
            </p>
            <Button asChild variant="outline" className="mt-6 gap-2">
              <Link to="/docs">
                API reference
                <ArrowRight className="size-4" />
              </Link>
            </Button>
          </div>

          <pre className="overflow-x-auto rounded-xl border bg-background p-5 text-sm leading-relaxed shadow-sm">
            <code>{`# Create a draft, then send it.
curl -X POST https://your-host/api/v1/drafts \\
  -H "Authorization: Bearer $XMAIL_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "mail_account_id": "$ACCOUNT",
    "from_address_id": "$SENDER",
    "to": [{ "email": "customer@example.com" }],
    "subject": "Your order shipped",
    "html_body": "<p>On its way.</p>"
  }'

curl -X POST https://your-host/api/v1/drafts/$DRAFT/send \\
  -H "Authorization: Bearer $XMAIL_TOKEN"`}</code>
          </pre>
        </div>
      </div>
    </section>
  )
}

function SelfHost() {
  return (
    <section className="mx-auto w-full max-w-6xl px-4 py-16">
      <div className="grid gap-10 lg:grid-cols-2 lg:items-center">
        <div>
          <h2 className="flex items-center gap-2 text-2xl font-semibold tracking-tight">
            <Server className="size-5" />
            Runs where you put it
          </h2>
          <p className="mt-4 text-muted-foreground text-pretty">
            A Go API, a background worker, PostgreSQL, Redis and a static frontend. Four containers
            and a reverse proxy — no managed service you cannot replace, and your mail stays in
            your database.
          </p>
          <Button asChild variant="outline" className="mt-6 gap-2">
            <Link to="/docs">
              Installation guide
              <ArrowRight className="size-4" />
            </Link>
          </Button>
        </div>

        <pre className="overflow-x-auto rounded-xl border bg-muted/40 p-5 text-sm leading-relaxed">
          <code>{`git clone <your-fork> x-mail
cd x-mail
cp .env.example .env     # set the two secrets
make docker-up           # → http://localhost:8180`}</code>
        </pre>
      </div>
    </section>
  )
}

function Closing() {
  return (
    <section className="border-t">
      <div className="mx-auto w-full max-w-6xl px-4 py-16 text-center">
        <h2 className="text-2xl font-semibold tracking-tight">Start with one account</h2>
        <p className="mx-auto mt-3 max-w-xl text-muted-foreground text-pretty">
          Connect a provider, add a sender address, and send yourself the first message. Adding the
          rest is the same three steps again.
        </p>
        <div className="mt-7 flex flex-wrap justify-center gap-3">
          <Button asChild size="lg" className="gap-2">
            <Link to="/register">
              Create an account
              <ArrowRight className="size-4" />
            </Link>
          </Button>
          <Button asChild size="lg" variant="outline">
            <Link to="/docs">
              Getting started
            </Link>
          </Button>
        </div>
      </div>
    </section>
  )
}
