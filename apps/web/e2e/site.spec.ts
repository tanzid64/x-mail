import { expect, test, type Page } from '@playwright/test'

/**
 * The home page and the documentation are the only parts of x-mail readable
 * without an account, so the thing worth testing is that they stay that way —
 * and that a guide nobody can navigate is caught here rather than by a reader.
 */

function newAccount() {
  const id = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
  return { email: `site-${id}@example.com`, password: 'a-very-good-password' }
}

async function signUp(page: Page) {
  const account = newAccount()
  await page.goto('/register')
  await page.fill('#name', 'Site Tester')
  await page.fill('#email', account.email)
  await page.fill('#password', account.password)
  await page.click('button[type=submit]')
  await page.waitForURL('**/inbox')
  return account
}

test('the home page is readable without an account', async ({ page }) => {
  await page.goto('/')

  await expect(page.getByRole('heading', { level: 1 })).toContainText('One mailbox')
  // The model is the thing a newcomer most needs, so it is on the page itself.
  await expect(page.getByRole('heading', { name: 'A user is not an email account' })).toBeVisible()

  await expect(page.getByRole('link', { name: 'Sign in' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Create account' })).toBeVisible()
})

test('a signed-in visitor goes to their inbox instead of the pitch', async ({ page }) => {
  await signUp(page)

  await page.goto('/')
  await page.waitForURL('**/inbox')

  // The documentation stays reachable, and offers the way back.
  await page.goto('/docs')
  await expect(page.getByRole('heading', { level: 1 })).toContainText('Overview')
  await expect(page.getByRole('link', { name: 'Go to inbox' })).toBeVisible()
})

test('every documentation page is reachable and renders', async ({ page }) => {
  await page.goto('/docs')

  const sidebar = page.getByRole('navigation', { name: 'Documentation' })
  const titles = await sidebar.getByRole('link').allInnerTexts()
  expect(titles.length).toBeGreaterThan(10)

  for (const title of titles) {
    await sidebar.getByRole('link', { name: title, exact: true }).click()
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(title)
    await expect(page.getByText('Page not found')).toHaveCount(0)
  }
})

test('an unknown documentation page says so instead of rendering blank', async ({ page }) => {
  await page.goto('/docs/no-such-page')

  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Page not found')
  await expect(page.getByRole('link', { name: 'Back to the documentation' })).toBeVisible()
})

test('the guide covers installation, the interface and the API', async ({ page }) => {
  // Each leg of the journey a reader is promised, spot-checked for content
  // rather than merely for a heading.
  await page.goto('/docs/installation')
  await expect(page.getByText('make docker-up').first()).toBeVisible()

  await page.goto('/docs/mailbox')
  await expect(page.getByRole('heading', { name: 'Folders' })).toBeVisible()

  await page.goto('/docs/api-sending')
  await expect(page.getByText('POST', { exact: true }).first()).toBeVisible()
  await expect(page.getByText('/drafts/{id}/send').first()).toBeVisible()
})

test('the reference explains the two message bodies', async ({ page }) => {
  // Rendering html_body is the most likely way to turn this API into an XSS
  // vector, so the warning must actually be on the page.
  await page.goto('/docs/api-mailbox')

  const warning = page.getByText('Two bodies, and only one is safe to render')
  await expect(warning).toBeVisible()
  await expect(page.getByText('html_sanitized').first()).toBeVisible()
})

test('documentation code blocks can be copied', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.goto('/docs/installation')

  const block = page.locator('figure').filter({ hasText: 'make docker-up' }).first()
  await block.hover()
  await block.getByRole('button', { name: 'Copy to clipboard' }).click()

  const copied = await page.evaluate(() => navigator.clipboard.readText())
  expect(copied).toContain('make docker-up')
})
