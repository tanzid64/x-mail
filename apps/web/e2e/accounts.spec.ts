import { expect, test, type Page } from '@playwright/test'

/** The stub provider accepts any key with this prefix and rejects the rest. */
const VALID_KEY = 're_e2e_valid_key_123456'
const INVALID_KEY = 're_definitely_not_valid'

function newAccount() {
  const id = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
  return { email: `accounts-${id}@example.com`, password: 'a-very-good-password' }
}

async function signUp(page: Page) {
  const account = newAccount()
  await page.goto('/register')
  await page.fill('#name', 'Account Tester')
  await page.fill('#email', account.email)
  await page.fill('#password', account.password)
  await page.click('button[type=submit]')
  await page.waitForURL('**/inbox')
  return account
}

async function connectAccount(page: Page, name: string, apiKey = VALID_KEY) {
  await page.getByRole('button', { name: 'Add mail account' }).first().click()
  await page.fill('#account-name', name)
  await page.fill('#account-key', apiKey)
  await page.getByRole('button', { name: 'Connect' }).click()
}

test('connecting a mail account stores it and syncs its domains', async ({ page }) => {
  await signUp(page)
  await page.goto('/settings/accounts')

  await expect(page.getByText('No mail accounts yet')).toBeVisible()

  await connectAccount(page, 'Work')

  // The dialog closes only once the provider accepted the key.
  await expect(page.getByRole('dialog')).toBeHidden()
  await expect(page.getByRole('link', { name: /^Work/ })).toBeVisible()
  await expect(page.getByText('Default').first()).toBeVisible()

  // The masked hint is shown; the key itself must never appear.
  await expect(page.getByText('re_e2e…3456')).toBeVisible()
  await expect(page.locator('body')).not.toContainText(VALID_KEY)

  await page.getByRole('link', { name: /^Work/ }).click()
  await page.waitForURL(/\/settings\/accounts\/[0-9a-f-]+$/)

  // Domains were pulled from the provider when the account was connected.
  await expect(page.getByRole('heading', { name: 'company.test' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'pending.test' })).toBeVisible()
  await expect(page.getByText('Verified')).toBeVisible()
  await expect(page.getByText('Pending DNS')).toBeVisible()
})

test('a key the provider rejects is reported on the field and nothing is saved', async ({ page }) => {
  await signUp(page)
  await page.goto('/settings/accounts')

  await connectAccount(page, 'Bad Key', INVALID_KEY)

  await expect(page.getByText('The provider rejected this API key.')).toBeVisible()
  // The dialog stays open so the key can be corrected.
  await expect(page.getByRole('dialog')).toBeVisible()

  await page.getByRole('button', { name: 'Cancel' }).click()
  await expect(page.getByText('No mail accounts yet')).toBeVisible()
})

test('the switcher lists every account and can select one or all', async ({ page }) => {
  await signUp(page)
  await page.goto('/settings/accounts')

  await connectAccount(page, 'Personal')
  await expect(page.getByRole('dialog')).toBeHidden()
  await connectAccount(page, 'Work')
  await expect(page.getByRole('dialog')).toBeHidden()

  const switcher = page.getByRole('button', { name: 'Switch mail account' })
  await expect(switcher).toContainText('All accounts')

  await switcher.click()
  await page.getByRole('menuitem', { name: 'Work' }).click()
  await expect(switcher).toContainText('Work')

  // The selection survives a reload, so reopening returns to the same mailbox.
  await page.reload()
  await expect(page.getByRole('button', { name: 'Switch mail account' })).toContainText('Work')

  await page.getByRole('button', { name: 'Switch mail account' }).click()
  await page.getByRole('menuitem', { name: 'All accounts' }).click()
  await expect(page.getByRole('button', { name: 'Switch mail account' })).toContainText('All accounts')
})

test('a sender address can be added to a verified domain', async ({ page }) => {
  await signUp(page)
  await page.goto('/settings/accounts')
  await connectAccount(page, 'Support')
  await expect(page.getByRole('dialog')).toBeHidden()

  await page.getByRole('link', { name: /^Support/ }).click()
  await page.waitForURL(/\/settings\/accounts\/[0-9a-f-]+$/)

  const verified = page.getByRole('region', { name: 'company.test' })
  await verified.getByLabel('Address').fill('hello@company.test')
  await verified.getByLabel('Display name').fill('Hello')
  await verified.getByRole('button', { name: 'Add' }).click()

  await expect(verified.getByText('hello@company.test')).toBeVisible()

  // An address on a different domain is refused, with the reason shown.
  await verified.getByLabel('Address').fill('nope@elsewhere.test')
  await verified.getByRole('button', { name: 'Add' }).click()
  await expect(page.getByText('This address must be on the selected domain.')).toBeVisible()
})

test('removing an account clears it from the switcher', async ({ page }) => {
  await signUp(page)
  await page.goto('/settings/accounts')
  await connectAccount(page, 'Temporary')
  await expect(page.getByRole('dialog')).toBeHidden()

  await page.getByRole('link', { name: /^Temporary/ }).click()
  await page.waitForURL(/\/settings\/accounts\/[0-9a-f-]+$/)

  await page.getByRole('button', { name: 'Remove account' }).click()
  await page.getByRole('button', { name: 'Remove', exact: true }).click()

  await page.waitForURL('**/settings/accounts')
  await expect(page.getByText('No mail accounts yet')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Switch mail account' })).toContainText('No accounts yet')
})

test('testing a key, syncing and rechecking all report what happened', async ({ page }) => {
  await signUp(page)
  await page.goto('/settings/accounts')

  await connectAccount(page, 'Feedback')
  await expect(page.getByRole('dialog')).toBeHidden()
  await expect(page.getByText('“Feedback” is connected.', { exact: false })).toBeVisible()

  await page.getByRole('link', { name: /^Feedback/ }).click()
  await page.waitForURL(/\/settings\/accounts\/[0-9a-f-]+$/)

  // Each of these used to spin briefly and then say nothing at all, which is
  // indistinguishable from a button that does not work.
  await page.getByRole('button', { name: 'Test key' }).click()
  await expect(page.getByText('The API key works.')).toBeVisible()

  await page.getByRole('button', { name: 'Sync from provider' }).click()
  await expect(page.getByText('Synced 2 domains from Resend.')).toBeVisible()

  await page
    .getByRole('region', { name: 'pending.test' })
    .getByRole('button', { name: 'Recheck DNS' })
    .click()
  await expect(page.getByText('pending.test is still pending.', { exact: false })).toBeVisible()

  await page
    .getByRole('region', { name: 'company.test' })
    .getByRole('button', { name: 'Recheck DNS' })
    .click()
  await expect(page.getByText('company.test is verified.')).toBeVisible()
})
