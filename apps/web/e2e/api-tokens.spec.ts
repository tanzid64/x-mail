import { expect, request as playwrightRequest, test, type Page } from '@playwright/test'

/** The API, reached directly — the way a third party would, with no browser. */
const API = 'http://localhost:8091/api/v1'

function newAccount() {
  const id = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
  return { email: `tokens-${id}@example.com`, password: 'a-very-good-password' }
}

async function signUp(page: Page) {
  const account = newAccount()
  await page.goto('/register')
  await page.fill('#name', 'Token Tester')
  await page.fill('#email', account.email)
  await page.fill('#password', account.password)
  await page.click('button[type=submit]')
  await page.waitForURL('**/inbox')
  return account
}

/** Creates a token through the UI and returns the secret shown once. */
async function createToken(page: Page, name: string, expiry: string, access?: string) {
  // The trigger and the submit button share a label, so the dialog has to be
  // shut before "Create token" unambiguously means the trigger.
  await expect(page.getByRole('dialog')).toBeHidden()
  await page.getByRole('button', { name: 'Create token' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Name').fill(name)

  await dialog.getByLabel('Expires').click()
  await page.getByRole('option', { name: expiry, exact: true }).click()

  if (access) {
    await dialog.getByLabel('Access').click()
    await page.getByRole('option', { name: access }).click()
  }

  await dialog.getByRole('button', { name: 'Create token' }).click()

  const secret = await dialog.getByTestId('token-secret').inputValue()
  expect(secret).toMatch(/^xm_/)
  return secret
}

test('a token is shown once, then only ever as a hint', async ({ page }) => {
  await signUp(page)
  await page.goto('/settings/api-tokens')

  await expect(page.getByText('No API tokens yet')).toBeVisible()

  const secret = await createToken(page, 'CI deploy', '30 days')

  // Dismissing the dialog is the user's call, since the secret is nowhere else.
  await page.getByRole('button', { name: 'Done' }).click()
  await expect(page.getByRole('dialog')).toBeHidden()

  const list = page.getByTestId('api-token-list')
  await expect(list.getByText('CI deploy')).toBeVisible()
  await expect(list.getByText(/Expires in (29|30) days/)).toBeVisible()

  // The secret is gone from the page; only the masked hint remains.
  await expect(page.locator('body')).not.toContainText(secret)
  await expect(page.getByText(secret.slice(0, 7))).toBeVisible()

  // And it survives a reload as a hint, never as the secret.
  await page.reload()
  await expect(page.locator('body')).not.toContainText(secret)
})

test('all four expiry options are offered, and the extremes are stored', async ({ page }) => {
  await signUp(page)
  await page.goto('/settings/api-tokens')

  await page.getByRole('button', { name: 'Create token' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Expires').click()

  // Exactly these four, and nothing else.
  for (const option of ['15 days', '30 days', '1 year', 'Never expires']) {
    await expect(page.getByRole('option', { name: option, exact: true })).toBeVisible()
  }
  await expect(page.getByRole('option')).toHaveCount(4)

  // Carry straight on with the shortest rather than reopening the dialog.
  await page.getByRole('option', { name: '15 days', exact: true }).click()
  await dialog.getByLabel('Name').fill('Fifteen')
  await dialog.getByRole('button', { name: 'Create token' }).click()
  await expect(dialog.getByTestId('token-secret')).toBeVisible()
  await page.getByRole('button', { name: 'Done' }).click()

  await createToken(page, 'Forever', 'Never expires')
  await page.getByRole('button', { name: 'Done' }).click()

  const list = page.getByTestId('api-token-list')
  await expect(list.getByText(/Expires in (14|15) days/)).toBeVisible()
  await expect(list.getByText('Never expires')).toBeVisible()
})

test('a token authenticates the API with no browser session at all', async ({ page }) => {
  await signUp(page)
  await page.goto('/settings/api-tokens')
  const secret = await createToken(page, 'Integration', '1 year')
  await page.getByRole('button', { name: 'Done' }).click()

  // A fresh context with no cookies: the token is the only credential.
  const api = await playwrightRequest.newContext()

  const me = await api.get(`${API}/me`, { headers: { Authorization: `Bearer ${secret}` } })
  expect(me.status()).toBe(200)
  expect((await me.json()).user.email).toContain('tokens-')

  for (const path of ['/mail-accounts', '/threads', '/drafts', '/labels']) {
    const response = await api.get(`${API}${path}`, {
      headers: { Authorization: `Bearer ${secret}` },
    })
    expect(response.status(), `GET ${path}`).toBe(200)
  }

  // Without it, the same requests are refused.
  expect((await api.get(`${API}/me`)).status()).toBe(401)
  expect((await api.get(`${API}/me`, { headers: { Authorization: 'Bearer xm_nonsense' } })).status()).toBe(401)

  await api.dispose()
})

test('revoking a token stops it working immediately', async ({ page }) => {
  await signUp(page)
  await page.goto('/settings/api-tokens')
  const secret = await createToken(page, 'Leaked key', '30 days')
  await page.getByRole('button', { name: 'Done' }).click()

  const api = await playwrightRequest.newContext()
  expect((await api.get(`${API}/me`, { headers: { Authorization: `Bearer ${secret}` } })).status()).toBe(200)

  await page.getByRole('button', { name: 'Revoke Leaked key' }).click()
  await page.getByRole('button', { name: 'Revoke', exact: true }).click()

  await expect(page.getByText('“Leaked key” was revoked.')).toBeVisible()
  await expect(page.getByText('No API tokens yet')).toBeVisible()

  const after = await api.get(`${API}/me`, { headers: { Authorization: `Bearer ${secret}` } })
  expect(after.status()).toBe(401)
  expect((await after.json()).error.message).toContain('revoked')

  await api.dispose()
})

test('a token cannot issue another token', async ({ page }) => {
  await signUp(page)
  await page.goto('/settings/api-tokens')
  const secret = await createToken(page, 'Limited', '15 days')
  await page.getByRole('button', { name: 'Done' }).click()

  const api = await playwrightRequest.newContext()

  // The escalation this blocks: a 15-day key minting itself a permanent one.
  const minted = await api.post(`${API}/api-tokens`, {
    headers: { Authorization: `Bearer ${secret}` },
    data: { name: 'Forever', expires_in: 'never' },
  })
  expect(minted.status()).toBe(403)

  await api.dispose()
})
