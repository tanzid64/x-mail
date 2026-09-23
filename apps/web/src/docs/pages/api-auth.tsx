import { Endpoint, Field, Fields, Part } from '@/components/docs/endpoint'
import { Callout, Code, CodeBlock, H2, Lede, P } from '@/components/docs/prose'

export function ApiAuthPage() {
  return (
    <>
      <Lede>
        Signing in, the signed-in user, and managing the tokens that let something other than a
        browser call this API.
      </Lede>

      <H2 id="auth">Authentication</H2>

      <P>
        These set and clear the session cookie. A service integrating with x-mail wants an API
        token instead — it does not need any of these.
      </P>

      <Endpoint id="register" method="POST" path="/auth/register">
        <P>Creates a user and signs them in. Public.</P>
        <Part title="Body">
          <Fields>
            <Field name="email" type="string" required>
              Must not already be registered.
            </Field>
            <Field name="password" type="string" required>
              At least 12 characters.
            </Field>
            <Field name="name" type="string" required>
              Up to 100 characters.
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <P>
            <Code>201</Code> with <Code>{'{ "user": … }'}</Code>, and sets the session cookie. A
            confirmation email is sent.
          </P>
          <P>
            <Code>409</Code> if the address is taken, <Code>422</Code> for a password that is too
            short, <Code>429</Code> after 5 attempts in an hour from one address.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="login" method="POST" path="/auth/login">
        <P>Signs in and sets the session cookie. Public.</P>
        <Part title="Body">
          <Fields>
            <Field name="email" type="string" required>
              —
            </Field>
            <Field name="password" type="string" required>
              —
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <P>
            <Code>200</Code> with the user, or <Code>401</Code>.
          </P>
          <P>
            A wrong password and an unknown address give the identical answer, so the response
            cannot be used to work out who has an account. Limited to 10 a minute per address.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="logout" method="POST" path="/auth/logout">
        <P>
          Ends this session and clears the cookie. Other sessions are untouched. Answers{' '}
          <Code>204</Code> whether or not you were signed in.
        </P>
      </Endpoint>

      <Endpoint id="forgot-password" method="POST" path="/auth/forgot-password">
        <P>Sends a reset link. Public.</P>
        <Part title="Body">
          <Fields>
            <Field name="email" type="string" required>
              —
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <P>
            Always <Code>200</Code> with the same message, registered or not — otherwise this
            endpoint would be a way to enumerate accounts. 5 an hour per address.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="reset-password" method="POST" path="/auth/reset-password">
        <P>Sets a new password using the token from the emailed link. Public.</P>
        <Part title="Body">
          <Fields>
            <Field name="token" type="string" required>
              From the link. Single use.
            </Field>
            <Field name="password" type="string" required>
              At least 12 characters.
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <P>
            <Code>200</Code>, or <Code>400</Code> if the token is wrong, used or expired.
          </P>
          <P>
            Every session is revoked, including this browser's. A reset may be driven by an
            attacker who already holds a session, so none may survive it.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="verify-email" method="POST" path="/auth/verify-email">
        <P>Confirms an address from the emailed link. Public.</P>
        <Part title="Body">
          <Fields>
            <Field name="token" type="string" required>
              Consumed on use, so a link cannot be replayed.
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <P>
            <Code>200</Code> with the updated user, or <Code>400</Code>.
          </P>
        </Part>
      </Endpoint>

      <H2 id="me">The signed-in user</H2>

      <Endpoint id="get-me" method="GET" path="/me">
        <P>
          Who this credential belongs to. The first call a client makes, and the cheapest way to
          check a token works.
        </P>
        <Part title="Returns">
          <CodeBlock>{`{
  "user": {
    "id": "8f14e45f-ceea-467a-9b8f-6f2d3c4a5b6c",
    "email": "you@example.com",
    "name": "Your Name",
    "avatar_url": null,
    "email_verified_at": "2026-01-10T12:00:00Z",
    "is_admin": false,
    "is_active": true,
    "created_at": "2026-01-01T09:00:00Z",
    "updated_at": "2026-01-10T12:00:00Z"
  }
}`}</CodeBlock>
          <P>
            <Code>email_verified_at</Code> is <Code>null</Code> until the address is confirmed.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="patch-me" method="PATCH" path="/me">
        <P>Updates your profile. Omitted fields are left alone.</P>
        <Part title="Body">
          <Fields>
            <Field name="name" type="string">
              Up to 100 characters.
            </Field>
            <Field name="avatar_url" type="string | null">
              Send <Code>null</Code> to remove it.
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <P>
            <Code>200</Code> with the updated user.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="change-password" method="POST" path="/me/password">
        <P>
          Changes your password. <strong>Session only</strong> — a token gets <Code>403</Code>.
        </P>
        <Part title="Body">
          <Fields>
            <Field name="current_password" type="string" required>
              —
            </Field>
            <Field name="new_password" type="string" required>
              12–256 characters.
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <P>
            <Code>200</Code>. Every other session is signed out; this one survives, because a
            voluntary change should not log you out of the tab you are in.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="resend-verification" method="POST" path="/me/resend-verification">
        <P>
          Sends the confirmation email again. <Code>200</Code>.
        </P>
      </Endpoint>

      <H2 id="tokens">API tokens</H2>

      <Callout tone="warning" title="Session only">
        <P>
          All three refuse API tokens with <Code>403</Code>. A token that could mint another would
          make every expiry meaningless.
        </P>
      </Callout>

      <Endpoint id="list-tokens" method="GET" path="/api-tokens">
        <P>Your live tokens, newest first. Revoked ones are not listed.</P>
        <Part title="Returns">
          <CodeBlock>{`{
  "api_tokens": [
    {
      "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
      "name": "CI deploy",
      "hint": "xm_7Kd9…qR4",
      "mail_account_id": null,
      "expires_at": "2026-02-09T12:00:00Z",
      "expires_in": "30d",
      "last_used_at": "2026-01-12T08:14:00Z",
      "revoked_at": null,
      "created_at": "2026-01-10T12:00:00Z"
    }
  ]
}`}</CodeBlock>
          <P>
            The secret is never here — only <Code>hint</Code>. <Code>mail_account_id</Code> is{' '}
            <Code>null</Code> when the token reaches every account.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="create-token" method="POST" path="/api-tokens">
        <P>Creates a token and returns the secret, once.</P>
        <Part title="Body">
          <Fields>
            <Field name="name" type="string" required>
              Up to 100 characters. How you will recognise it later.
            </Field>
            <Field name="expires_in" type="string" required>
              One of <Code>"15d"</Code>, <Code>"30d"</Code>, <Code>"1y"</Code>,{' '}
              <Code>"never"</Code>. Anything else is <Code>422</Code>.
            </Field>
            <Field name="mail_account_id" type="uuid | null">
              Limits the token to one account. Omit or send <Code>null</Code> for all of them,
              including accounts added later.
            </Field>
          </Fields>
        </Part>
        <Part title="Returns">
          <CodeBlock>{`{
  "api_token": { "id": "...", "name": "CI deploy", "hint": "xm_7Kd9…qR4", ... },
  "token": "xm_7Kd9x2QmVbN4pLsR8tYwZ1aC3eF5gH6jK0mN9oP2qR4"
}`}</CodeBlock>
          <P>
            <Code>201</Code>. <Code>token</Code> appears here and nowhere else, ever again — only
            its hash is stored. Capture it now.
          </P>
          <P>
            <Code>422</Code> for an unsupported <Code>expires_in</Code>, or a{' '}
            <Code>mail_account_id</Code> that is not yours. 20 an hour per address.
          </P>
        </Part>
      </Endpoint>

      <Endpoint id="revoke-token" method="DELETE" path="/api-tokens/{tokenID}">
        <P>
          Revokes a token. <Code>204</Code>, or <Code>404</Code> if it is not yours.
        </P>
        <Part title="Notes">
          <P>
            Effective on the token's next request, with no grace period and no undo. Anything using
            it starts getting <Code>401</Code> with a message naming revocation specifically, so an
            integrator is not sent hunting for a typo.
          </P>
        </Part>
      </Endpoint>
    </>
  )
}
