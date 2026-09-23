import { createHmac, randomUUID } from 'node:crypto'

import { expect, test, type Page } from '@playwright/test'

const VALID_KEY = 're_e2e_valid_key_123456'
const API_BASE = 'http://localhost:8091/api/v1'

/** A base64 secret, in the "whsec_" form a provider hands out. */
const WEBHOOK_SECRET = `whsec_${Buffer.from('e2e-webhook-signing-secret').toString('base64')}`

/**
 * Signs a webhook the way Svix (and therefore Resend) does: HMAC-SHA256 over
 * "{id}.{timestamp}.{body}".
 */
function signedHeaders(body: string) {
  const id = `msg_${randomUUID()}`
  const timestamp = Math.floor(Date.now() / 1000).toString()
  const secret = Buffer.from(WEBHOOK_SECRET.slice('whsec_'.length), 'base64')
  const signature = createHmac('sha256', secret).update(`${id}.${timestamp}.${body}`).digest('base64')

  return {
    'content-type': 'application/json',
    'svix-id': id,
    'svix-timestamp': timestamp,
    'svix-signature': `v1,${signature}`,
  }
}

/** Sets up an account with a verified address and a webhook secret. */
async function setUpReceivingAccount(page: Page) {
  const user = `inbound-${Date.now()}-${Math.random().toString(36).slice(2, 8)}@example.com`
  const address = `support-${Date.now()}@company.test`

  await page.goto('/register')
  await page.fill('#name', 'Inbound Tester')
  await page.fill('#email', user)
  await page.fill('#password', 'a-very-good-password')
  await page.click('button[type=submit]')
  await page.waitForURL('**/inbox')

  await page.goto('/settings/accounts')
  await page.getByRole('button', { name: 'Add mail account' }).first().click()
  await page.fill('#account-name', 'Support')
  await page.fill('#account-key', VALID_KEY)
  await page.getByRole('button', { name: 'Connect' }).click()
  await expect(page.getByRole('dialog')).toBeHidden()

  await page.getByRole('link', { name: /^Support/ }).click()
  await page.waitForURL(/\/settings\/accounts\/[0-9a-f-]+$/)
  const accountId = page.url().split('/').pop()!

  const verified = page.getByRole('region', { name: 'company.test' })
  await verified.getByLabel('Address').fill(address)
  await verified.getByRole('button', { name: 'Add' }).click()
  await expect(verified.getByText(address)).toBeVisible()

  await page.fill('#webhook-secret', WEBHOOK_SECRET)
  await page.getByRole('button', { name: 'Save secret' }).click()
  await expect(page.getByText('Signing secret saved.')).toBeVisible()

  return { accountId, address }
}

function receivedEvent(to: string, subject: string, messageId: string) {
  return {
    type: 'email.received',
    created_at: new Date().toISOString(),
    data: {
      email_id: `prov_${randomUUID()}`,
      from: 'Customer <customer@example.com>',
      to: [to],
      subject,
      html: '<p>My payment did not go through.</p><script>alert(1)</script>',
      text: 'My payment did not go through.',
      headers: [{ name: 'Message-Id', value: messageId }],
    },
  }
}

test('a signed webhook delivers mail into the inbox with scripts stripped', async ({ page, request }) => {
  const { accountId, address } = await setUpReceivingAccount(page)

  const body = JSON.stringify(receivedEvent(address, 'Payment issue', `<${randomUUID()}@example.com>`))
  const response = await request.post(`${API_BASE}/webhooks/resend/${accountId}`, {
    headers: signedHeaders(body),
    data: body,
  })
  expect(response.status()).toBe(200)

  // Processing is asynchronous, so the inbox catches up shortly after.
  await expect(async () => {
    await page.goto('/inbox')
    await expect(page.getByText('Payment issue')).toBeVisible({ timeout: 2000 })
  }).toPass({ timeout: 20_000 })

  await page.getByTestId('email-row').filter({ hasText: 'Payment issue' }).getByRole('link').click()
  await page.waitForURL(/\/thread\/[0-9a-f-]+$/)

  const frame = page.frameLocator('iframe[title="Message content"]').last()
  await expect(frame.locator('body')).toContainText('My payment did not go through')
  // The script tag must not survive into what the reader sees.
  await expect(frame.locator('script')).toHaveCount(0)
})

test('an unsigned or tampered webhook is rejected', async ({ page, request }) => {
  const { accountId, address } = await setUpReceivingAccount(page)

  const body = JSON.stringify(receivedEvent(address, 'Should not arrive', `<${randomUUID()}@example.com>`))

  // No signature at all.
  const unsigned = await request.post(`${API_BASE}/webhooks/resend/${accountId}`, {
    headers: { 'content-type': 'application/json' },
    data: body,
  })
  expect(unsigned.status()).toBe(401)

  // Signed, then the body swapped underneath the signature.
  const headers = signedHeaders(body)
  const tampered = JSON.stringify(
    receivedEvent(address, 'Tampered subject', `<${randomUUID()}@example.com>`),
  )
  const swapped = await request.post(`${API_BASE}/webhooks/resend/${accountId}`, {
    headers,
    data: tampered,
  })
  expect(swapped.status()).toBe(401)

  await page.goto('/inbox')
  await expect(page.getByText('Should not arrive')).toBeHidden()
  await expect(page.getByText('Tampered subject')).toBeHidden()
})

// Providers redeliver whenever they are unsure the first attempt landed.
test('the same webhook delivered twice produces one message', async ({ page, request }) => {
  const { accountId, address } = await setUpReceivingAccount(page)

  const body = JSON.stringify(receivedEvent(address, 'Only once please', `<${randomUUID()}@example.com>`))
  const headers = signedHeaders(body)

  for (let attempt = 0; attempt < 3; attempt++) {
    const response = await request.post(`${API_BASE}/webhooks/resend/${accountId}`, { headers, data: body })
    expect(response.status()).toBe(200)
  }

  await expect(async () => {
    await page.goto('/inbox')
    await expect(page.getByText('Only once please')).toBeVisible({ timeout: 2000 })
  }).toPass({ timeout: 20_000 })

  await expect(page.getByTestId('email-row').filter({ hasText: 'Only once please' })).toHaveCount(1)
})

test('an inbound reply joins the conversation it answers', async ({ page, request }) => {
  const { accountId, address } = await setUpReceivingAccount(page)

  const rootMessageId = `<${randomUUID()}@example.com>`
  const root = JSON.stringify(receivedEvent(address, 'Refund request', rootMessageId))
  await request.post(`${API_BASE}/webhooks/resend/${accountId}`, {
    headers: signedHeaders(root),
    data: root,
  })

  const reply = receivedEvent(address, 'Re: Refund request', `<${randomUUID()}@example.com>`)
  reply.data.headers = [
    { name: 'Message-Id', value: `<${randomUUID()}@example.com>` },
    { name: 'In-Reply-To', value: rootMessageId },
    { name: 'References', value: rootMessageId },
  ]
  const replyBody = JSON.stringify(reply)
  await request.post(`${API_BASE}/webhooks/resend/${accountId}`, {
    headers: signedHeaders(replyBody),
    data: replyBody,
  })

  await expect(async () => {
    await page.goto('/inbox')
    const row = page.getByTestId('email-row').filter({ hasText: 'Refund request' })
    await expect(row).toHaveCount(1)
    // Two messages in one conversation, not two conversations.
    await expect(row).toContainText('2')
  }).toPass({ timeout: 20_000 })
})

// The whole point of the websocket: mail appears without the reader doing
// anything. This test never reloads the page.
test('inbound mail appears live, without a reload', async ({ page, request }) => {
  const { accountId, address } = await setUpReceivingAccount(page)

  await page.goto('/inbox')
  await expect(page.getByText('Nothing here')).toBeVisible()

  // Wait for the socket to be established before sending, otherwise the event
  // could be published to nobody and the test would pass for the wrong reason.
  await page.waitForFunction(() => {
    const entries = performance.getEntriesByType('resource')
    return entries.some((entry) => entry.name.includes('/api/v1/ws'))
  }, undefined, { timeout: 10_000 }).catch(() => {
    // Resource timing does not always record websockets; the assertion below
    // is the real check either way.
  })

  const body = JSON.stringify(
    receivedEvent(address, 'Arrived while you watched', `<${randomUUID()}@example.com>`),
  )
  const response = await request.post(`${API_BASE}/webhooks/resend/${accountId}`, {
    headers: signedHeaders(body),
    data: body,
  })
  expect(response.status()).toBe(200)

  // No page.goto and no reload — the row has to arrive on its own.
  await expect(page.getByText('Arrived while you watched')).toBeVisible({ timeout: 20_000 })
})
