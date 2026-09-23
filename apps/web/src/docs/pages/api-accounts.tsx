import { Endpoint, Field, Fields, Part } from '@/components/docs/endpoint'
import { Callout, Code, CodeBlock, H2, Lede, P } from '@/components/docs/prose'

export function ApiAccountsPage() {
  return (
    <>
      <Lede>
        Mail accounts, the domains under them and the sender addresses under those. Everything
        else in the API hangs off a mail account, so this is usually where an integration starts.
      </Lede>

      <H2 id="accounts">Mail accounts</H2>

      <Endpoint id="list-accounts" method="GET" path="/mail-accounts">
        <P>Every account you own, default first and then by name.</P>
        <Part title="Returns">
          <CodeBlock>{`{
  "mail_accounts": [
    {
      "id": "8f14e45f-ceea-467a-9b8f-6f2d3c4a5b6c",
      "name": "Support",
      "provider": "resend",
      "provider_account_id": null,
      "api_key_hint": "re_123…wxyz",
      "webhook_secret_hint": "whs…9f2a",
      "status": "active",
      "status_message": "",
      "last_checked_at": "2026-01-12T08:00:00Z",
      "color": "emerald",
      "is_default": true,
      "is_active": true,
      "created_at": "2026-01-01T09:00:00Z",
      "updated_at": "2026-01-12T08:00:00Z"
    }
  ]
}`}</CodeBlock>
          <P>
            <Code>status</Code> is <Code>active</Code>, <Code>invalid_credentials</Code> or{' '}
            <Code>disabled</Code>. A token limited to one account sees only that one.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="create-account" method="POST" path="/mail-accounts">
        <P>Connects a provider account.</P>
        <Part title="Body">
          <Fields>
            <Field name="name" type="string" required>
              Up to 100 characters, unique among your accounts.
            </Field>
            <Field name="provider" type="string" required>
              <Code>"resend"</Code>.
            </Field>
            <Field name="api_key" type="string" required>
              Verified against the provider before anything is stored.
            </Field>
            <Field name="color" type="string">
              <Code>slate</Code>, <Code>emerald</Code>, <Code>blue</Code>, <Code>violet</Code>,{' '}
              <Code>amber</Code> or <Code>rose</Code>.
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <P>
            <Code>201</Code> with the account. The first account you connect becomes the default,
            and its domains are synced as part of connecting.
          </P>
          <P>
            <Code>422</Code> with <Code>fields.api_key</Code> set if the provider rejected the key.{' '}
            <Code>409</Code> if the name is taken. <Code>403</Code> for a token limited to one
            account — creating another would widen its own scope.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="get-account" method="GET" path="/mail-accounts/{id}">
        <P>
          One account. <Code>404</Code> if it is not yours.
        </P>
      </Endpoint>

      <Endpoint id="patch-account" method="PATCH" path="/mail-accounts/{id}">
        <P>Renames, recolours or deactivates. Omitted fields are left alone.</P>
        <Part title="Body">
          <Fields>
            <Field name="name" type="string">
              —
            </Field>
            <Field name="color" type="string">
              —
            </Field>
            <Field name="is_active" type="boolean">
              —
            </Field>
          </Fields>
        </Part>
      </Endpoint>

      <Endpoint id="delete-account" method="DELETE" path="/mail-accounts/{id}">
        <P>
          Removes the account. <Code>204</Code>.
        </P>
        <Part title="Notes">
          <Callout tone="warning" title="This deletes the mail">
            <P>
              Domains, addresses and every stored message go with it. Your other accounts are
              untouched, and if this was the default, the oldest remaining account takes over.
              There is no undo.
            </P>
          </Callout>
        </Part>
      </Endpoint>

      <Endpoint id="default-account" method="POST" path="/mail-accounts/{id}/default">
        <P>
          Makes this the default account. Returns <Code>mail_accounts</Code> — all of them, with
          the flag moved — since making one default necessarily un-defaults another.
        </P>
      </Endpoint>

      <Endpoint id="rotate-key" method="POST" path="/mail-accounts/{id}/api-key">
        <P>Replaces the stored provider key.</P>
        <Part title="Body">
          <Fields>
            <Field name="api_key" type="string" required>
              Verified before it replaces the old one, so a typo cannot lock the account out.
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <P>
            <Code>200</Code> with the account. Any recorded credential failure is cleared.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="check-account" method="POST" path="/mail-accounts/{id}/check">
        <P>Re-tests the stored credentials against the provider.</P>
        <Part title="Returns">
          <Callout tone="note" title="A rejected key is still a 200">
            <P>
              The verdict is recorded on the account and returned in the body, not signalled by the
              status code — the request succeeded, and what it found out is the answer. Read{' '}
              <Code>status</Code> and <Code>status_message</Code>:
            </P>
          </Callout>
          <CodeBlock>{`status = "active",              status_message = ""     → the key works
status = "invalid_credentials", status_message = "…"    → the provider rejected it
status = "active",              status_message = "…"    → could not reach the provider`}</CodeBlock>
          <P>
            The third case is a network problem, not a credentials problem, which is why it does
            not mark the account as broken.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="sync-domains" method="POST" path="/mail-accounts/{id}/sync-domains">
        <P>Pulls this account's domains from the provider and returns them.</P>
        <Part title="Notes">
          <P>
            This <strong>reconciles</strong>: domains removed at the provider are removed here too.
            To refresh one domain without touching the others, use{' '}
            <Code>POST /domains/{'{id}'}/refresh</Code>.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="webhook-secret" method="POST" path="/mail-accounts/{id}/webhook-secret">
        <P>Stores the signing secret for this account's provider callbacks.</P>
        <Part title="Body">
          <Fields>
            <Field name="secret" type="string" required>
              From your provider when you create the webhook.
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <P>
            <Code>200</Code>. Encrypted like the API key — a webhook secret is equally capable of
            letting someone inject mail into your mailbox. Inbound mail for this account is
            rejected until one is set, or a deployment-wide fallback is configured.
          </P>
        </Part>
      </Endpoint>

      <H2 id="domains">Domains</H2>

      <Endpoint id="list-domains" method="GET" path="/domains">
        <P>Domains across your accounts.</P>
        <Part title="Query">
          <Fields>
            <Field name="mail_account_id" type="uuid">
              Narrow to one account. Omit for all of them.
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <CodeBlock>{`{
  "domains": [
    {
      "id": "1b9d6bcd-bbfd-4b2d-9b5d-ab8dfbbd4bed",
      "mail_account_id": "8f14e45f-ceea-467a-9b8f-6f2d3c4a5b6c",
      "domain": "company.com",
      "provider_domain_id": "d91cd9bd-1176-453e-8fc1-35364d380206",
      "status": "verified",
      "region": "us-east-1",
      "records": [],
      "last_synced_at": "2026-01-12T08:00:00Z",
      "created_at": "2026-01-01T09:05:00Z",
      "updated_at": "2026-01-12T08:00:00Z"
    }
  ]
}`}</CodeBlock>
          <P>
            <Code>status</Code> is <Code>pending</Code>, <Code>verified</Code> or{' '}
            <Code>failed</Code>. <Code>records</Code> holds the DNS records still to publish, and
            is empty once verified.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="get-domain" method="GET" path="/domains/{id}">
        <P>One domain, with its outstanding DNS records.</P>
      </Endpoint>

      <Endpoint id="refresh-domain" method="POST" path="/domains/{id}/refresh">
        <P>
          Re-reads this domain from the provider and returns it. Touches only this domain — use it
          while waiting for DNS to propagate.
        </P>
      </Endpoint>

      <H2 id="addresses">Sender addresses</H2>

      <Endpoint id="list-addresses" method="GET" path="/addresses">
        <P>Every address you can send from.</P>
        <Part title="Query">
          <Fields>
            <Field name="mail_account_id" type="uuid">
              Narrow to one account.
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <CodeBlock>{`{
  "addresses": [
    {
      "id": "c9bf9e57-1685-4c89-bafb-ff5af830be8a",
      "domain_id": "1b9d6bcd-bbfd-4b2d-9b5d-ab8dfbbd4bed",
      "email": "support@company.com",
      "display_name": "Support",
      "is_default": true,
      "is_active": true,
      "domain": "company.com",
      "mail_account_id": "8f14e45f-ceea-467a-9b8f-6f2d3c4a5b6c",
      "created_at": "2026-01-01T09:10:00Z",
      "updated_at": "2026-01-01T09:10:00Z"
    }
  ]
}`}</CodeBlock>
          <P>
            <Code>domain</Code> and <Code>mail_account_id</Code> are denormalised onto the address
            so a sender picker can group them without further requests.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="create-address" method="POST" path="/domains/{id}/addresses">
        <P>Adds a sender address to a domain.</P>
        <Part title="Body">
          <Fields>
            <Field name="email" type="string" required>
              Must be on this domain.
            </Field>
            <Field name="display_name" type="string">
              The name recipients see. Up to 100 characters.
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <P>
            <Code>201</Code>. The first address on a domain becomes its default sender.
          </P>
          <P>
            <Code>422</Code> if the address is not on that domain — the provider would refuse to
            send it. <Code>409</Code> if the address is already in use anywhere in this
            installation: inbound routing resolves a recipient by address alone, so addresses are
            globally unique.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="get-address" method="GET" path="/addresses/{id}">
        <P>One address.</P>
      </Endpoint>

      <Endpoint id="patch-address" method="PATCH" path="/addresses/{id}">
        <P>Updates the display name or deactivates it.</P>
        <Part title="Body">
          <Fields>
            <Field name="display_name" type="string">
              —
            </Field>
            <Field name="is_active" type="boolean">
              An inactive address cannot be sent from.
            </Field>
          </Fields>
        </Part>
      </Endpoint>

      <Endpoint id="default-address" method="POST" path="/addresses/{id}/default">
        <P>Makes this the default sender for its domain.</P>
      </Endpoint>

      <Endpoint id="delete-address" method="DELETE" path="/addresses/{id}">
        <P>
          Removes the address. <Code>204</Code>. Mail already received at it stays in the mailbox.
        </P>
      </Endpoint>
    </>
  )
}
