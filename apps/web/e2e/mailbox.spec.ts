import { execFile } from 'node:child_process'
import { promisify } from 'node:util'

import { expect, test, type Page } from '@playwright/test'

const run = promisify(execFile)

const VALID_KEY = 're_e2e_valid_key_123456'
const E2E_DATABASE_URL = 'postgres://xmail:xmail@localhost:5433/xmail_e2e?sslmode=disable'

/**
 * Inbound mail does not arrive until the receiving phase, so the mailbox is
 * populated with the seed command — the same InsertMessage path the webhook
 * processor will use.
 */
async function seedMailbox(email: string) {
  await run('go', ['run', './cmd/seed', '-email', email], {
    cwd: '../api',
    env: {
      ...process.env,
      DATABASE_URL: E2E_DATABASE_URL,
      SESSION_SECRET: 'e2e-session-secret-that-is-long-enough-32',
      ENCRYPTION_MASTER_KEY: 'ZTJlLW1hc3Rlci1rZXktZXhhY3RseS0zMi1ieXRlcyE=',
      APP_ENV: 'test',
    },
    timeout: 120_000,
  })
}

/** Registers a user, connects an account and fills the mailbox. */
async function signUpWithMail(page: Page) {
  const email = `mailbox-${Date.now()}-${Math.random().toString(36).slice(2, 8)}@example.com`

  await page.goto('/register')
  await page.fill('#name', 'Mailbox Tester')
  await page.fill('#email', email)
  await page.fill('#password', 'a-very-good-password')
  await page.click('button[type=submit]')
  await page.waitForURL('**/inbox')

  await page.goto('/settings/accounts')
  await page.getByRole('button', { name: 'Add mail account' }).first().click()
  await page.fill('#account-name', 'Work')
  await page.fill('#account-key', VALID_KEY)
  await page.getByRole('button', { name: 'Connect' }).click()
  await expect(page.getByRole('dialog')).toBeHidden()

  await seedMailbox(email)
  return email
}

test('the inbox lists seeded conversations with unread state', async ({ page }) => {
  await signUpWithMail(page)

  await page.goto('/inbox')
  const rows = page.getByTestId('email-row')
  await expect(rows.first()).toBeVisible()

  await expect(page.getByText('Project update — week 12')).toBeVisible()
  await expect(page.getByText('Your invoice from AWS is ready')).toBeVisible()

  // Spam and trash must not leak into the inbox.
  await expect(page.getByText('Congratulations! You have won')).toBeHidden()
  await expect(page.getByText('Old thread we finished with')).toBeHidden()

  // The sidebar badge reflects unread conversations.
  await expect(page.locator('[data-sidebar="menu-badge"]').first()).toBeVisible()
})

test('folders show only their own mail', async ({ page }) => {
  await signUpWithMail(page)

  await page.goto('/spam')
  await expect(page.getByText('Congratulations! You have won')).toBeVisible()
  await expect(page.getByText('Project update — week 12')).toBeHidden()

  await page.goto('/trash')
  await expect(page.getByText('Old thread we finished with')).toBeVisible()

  await page.goto('/archive')
  await expect(page.getByText('Re: Payment issue on order 8812')).toBeVisible()

  await page.goto('/starred')
  await expect(page.getByText('Weekend plans?')).toBeVisible()
  await expect(page.getByText('Your invoice from AWS is ready')).toBeHidden()
})

test('opening a conversation shows its messages and marks it read', async ({ page }) => {
  await signUpWithMail(page)
  await page.goto('/inbox')

  const row = page.getByTestId('email-row').filter({ hasText: 'Project update — week 12' })
  await expect(row).toHaveAttribute('data-unread', 'true')

  await row.getByRole('link').click()
  await page.waitForURL(/\/thread\/[0-9a-f-]+$/)

  await expect(page.getByRole('heading', { name: 'Project update — week 12' })).toBeVisible()
  await expect(page.getByText('3 messages')).toBeVisible()

  // The newest message is expanded; the body renders in a sandboxed frame.
  const frame = page.frameLocator('iframe[title="Message content"]').last()
  await expect(frame.locator('body')).toContainText("I'll send the doc beforehand")

  await page.goto('/inbox')
  await expect(
    page.getByTestId('email-row').filter({ hasText: 'Project update — week 12' }),
  ).toHaveAttribute('data-unread', 'false')
})

test('archiving from the toolbar moves a conversation out of the inbox', async ({ page }) => {
  await signUpWithMail(page)
  await page.goto('/inbox')

  const row = page.getByTestId('email-row').filter({ hasText: 'Your invoice from AWS is ready' })
  await row.getByRole('checkbox').check()
  await expect(page.getByText('1 selected')).toBeVisible()

  await page.getByRole('button', { name: 'Archive' }).click()
  await expect(page.getByText('Your invoice from AWS is ready')).toBeHidden()

  await page.goto('/archive')
  await expect(page.getByText('Your invoice from AWS is ready')).toBeVisible()
})

test('select-all applies a bulk action to every conversation in view', async ({ page }) => {
  await signUpWithMail(page)
  await page.goto('/inbox')

  const before = await page.getByTestId('email-row').count()
  expect(before).toBeGreaterThan(1)

  await page.getByRole('checkbox', { name: 'Select all conversations' }).check()
  await expect(page.getByText(`${before} selected`)).toBeVisible()

  await page.getByRole('button', { name: 'Move to trash' }).click()
  await expect(page.getByText('Nothing here')).toBeVisible()

  await page.goto('/trash')
  await expect(page.getByTestId('email-row')).toHaveCount(before + 1)
})

test('starring from a row is reflected in the starred folder', async ({ page }) => {
  await signUpWithMail(page)
  await page.goto('/inbox')

  const row = page.getByTestId('email-row').filter({ hasText: 'Contract for review' })
  await row.getByRole('button', { name: 'Star conversation' }).click()

  await page.goto('/starred')
  await expect(page.getByText('Contract for review')).toBeVisible()
})
