import { execFile } from 'node:child_process'
import { promisify } from 'node:util'

import { expect, test, type Page } from '@playwright/test'

const run = promisify(execFile)

const VALID_KEY = 're_e2e_valid_key_123456'
const E2E_DATABASE_URL = 'postgres://xmail:xmail@localhost:5433/xmail_e2e?sslmode=disable'

/** Fills the mailbox, since inbound mail does not arrive until a later phase. */
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

/**
 * Registers a user, connects an account and adds a verified sender address —
 * everything a message needs before it can be sent.
 */
async function signUpReadyToSend(page: Page) {
  const email = `compose-${Date.now()}-${Math.random().toString(36).slice(2, 8)}@example.com`

  await page.goto('/register')
  await page.fill('#name', 'Compose Tester')
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

  // company.test is the stub provider's verified domain.
  await page.getByText('Work').first().click()
  await page.waitForURL(/\/settings\/accounts\/[0-9a-f-]+$/)

  const verified = page.getByRole('region', { name: 'company.test' })
  const sender = `sender-${Date.now()}@company.test`
  await verified.getByLabel('Address').fill(sender)
  await verified.getByLabel('Display name').fill('Compose Tester')
  await verified.getByRole('button', { name: 'Add' }).click()
  await expect(page.getByText(sender)).toBeVisible()

  return { email, sender }
}

/**
 * Opens the compose window and returns it, so assertions are scoped to the
 * dialog rather than the whole page — "Send" would otherwise also match the
 * verify-email banner's "Resend".
 */
async function openCompose(page: Page) {
  await page.getByRole('button', { name: 'Compose' }).click()
  const dialog = page.getByRole('dialog', { name: 'New message' })
  await expect(dialog).toBeVisible()
  return dialog
}

test('a message can be composed and sent', async ({ page }) => {
  await signUpReadyToSend(page)
  await page.goto('/inbox')
  const dialog = await openCompose(page)

  await page.fill('#compose-to', 'john@example.com')
  await page.keyboard.press('Enter')
  await page.fill('#compose-subject', 'Hello from the e2e suite')
  await page.getByLabel('Message body').fill('This is the body of the message.')

  await dialog.getByRole('button', { name: 'Send' }).click()
  await expect(dialog).toBeHidden()

  // The message lands in Sent. Sending is asynchronous, so the row appears
  // first and the worker's result follows.
  await page.goto('/sent')
  await expect(page.getByText('Hello from the e2e suite')).toBeVisible()

  await page.getByTestId('email-row').filter({ hasText: 'Hello from the e2e suite' }).getByRole('link').click()
  await page.waitForURL(/\/thread\/[0-9a-f-]+$/)

  // "Sent" rather than "Sending" is the proof that the worker really did hand
  // the message to the provider, not just that a row was written.
  await expect(async () => {
    await page.reload()
    await expect(page.getByTestId('send-status')).toContainText('Sent')
  }).toPass({ timeout: 20_000 })
})

test('a draft is autosaved and reopens with its content', async ({ page }) => {
  await signUpReadyToSend(page)
  await page.goto('/inbox')
  const dialog = await openCompose(page)

  await page.fill('#compose-to', 'draft@example.com')
  await page.keyboard.press('Enter')
  await page.fill('#compose-subject', 'Saved automatically')
  await page.getByLabel('Message body').fill('Half-written thought.')

  // Autosave is debounced; "Draft saved" is the signal that it landed.
  await expect(dialog.getByText('Draft saved')).toBeVisible({ timeout: 15_000 })

  await dialog.getByRole('button', { name: 'Close' }).click()
  await page.goto('/drafts')

  const row = page.getByTestId('draft-row').filter({ hasText: 'Saved automatically' })
  await expect(row).toBeVisible()
  await row.click()

  const reopened = page.getByRole('dialog', { name: 'New message' })
  await expect(reopened).toBeVisible()
  await expect(reopened.locator('#compose-subject')).toHaveValue('Saved automatically')
  // Scoped to the dialog: the draft row behind it shows the same address.
  await expect(reopened.getByText('draft@example.com')).toBeVisible()
})

test('sending without a recipient is refused', async ({ page }) => {
  await signUpReadyToSend(page)
  await page.goto('/inbox')
  const dialog = await openCompose(page)

  await page.fill('#compose-subject', 'Nobody to send to')
  await dialog.getByRole('button', { name: 'Send' }).click()

  await expect(dialog.getByText('Add at least one recipient.')).toBeVisible()
  // The window stays open so the mistake can be corrected.
  await expect(dialog).toBeVisible()
})

test('an attachment can be added to a draft and removed again', async ({ page }) => {
  await signUpReadyToSend(page)
  await page.goto('/inbox')
  await openCompose(page)

  await page.setInputFiles('input[aria-label="Attach files"]', {
    name: 'notes.txt',
    mimeType: 'text/plain',
    buffer: Buffer.from('some attached notes'),
  })

  const chip = page.getByTestId('attachment-chip')
  await expect(chip).toBeVisible()
  await expect(chip).toContainText('notes.txt')

  await page.getByRole('button', { name: 'Remove notes.txt' }).click()
  await expect(page.getByTestId('attachment-chip')).toHaveCount(0)
})

test('replying prefills the recipient, subject and quoted original', async ({ page }) => {
  const { email } = await signUpReadyToSend(page)
  await seedMailbox(email)

  await page.goto('/inbox')
  await page.getByTestId('email-row').filter({ hasText: 'Project update' }).getByRole('link').click()
  await page.waitForURL(/\/thread\/[0-9a-f-]+$/)

  await page.getByRole('button', { name: 'Reply', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'New message' })
  await expect(dialog).toBeVisible()

  // The sender becomes the recipient, and the subject gains one "Re:".
  // Scoped to the dialog: the thread behind it also shows the address.
  await expect(dialog.getByText('john@example.com')).toBeVisible()
  await expect(dialog.locator('#compose-subject')).toHaveValue(/^Re: Project update/)

  // The original is quoted in the body.
  await expect(dialog.getByLabel('Message body')).toContainText('wrote:')
})

test('reply all keeps the other recipients without adding your own address', async ({ page }) => {
  const { email, sender } = await signUpReadyToSend(page)
  await seedMailbox(email)

  await page.goto('/inbox')
  await page.getByTestId('email-row').filter({ hasText: 'Project update' }).getByRole('link').click()
  await page.waitForURL(/\/thread\/[0-9a-f-]+$/)

  await page.getByRole('button', { name: 'Reply all' }).click()
  await expect(page.getByRole('dialog', { name: 'New message' })).toBeVisible()

  const dialog = page.getByRole('dialog', { name: 'New message' })
  await expect(dialog.getByText('john@example.com')).toBeVisible()
  // Your own sending address must never end up in your own recipient list.
  await expect(dialog.getByText(sender, { exact: true })).toHaveCount(0)
})
