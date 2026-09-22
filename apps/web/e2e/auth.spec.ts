import { expect, test } from '@playwright/test'

/** Each run registers a fresh account so tests never collide on email. */
function newAccount() {
  const id = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
  return { email: `e2e-${id}@example.com`, password: 'a-very-good-password', name: 'E2E Tester' }
}

async function register(page: import('@playwright/test').Page, account: ReturnType<typeof newAccount>) {
  await page.goto('/register')
  await page.fill('#name', account.name)
  await page.fill('#email', account.email)
  await page.fill('#password', account.password)
  await page.click('button[type=submit]')
  await page.waitForURL('**/inbox')
}

test('signed-out visitors are sent to sign in, keeping where they were headed', async ({ page }) => {
  await page.goto('/')
  await expect(page).toHaveURL(/\/login\?redirect=/)
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible()
})

test('a new account can register, browse folders, sign out and sign back in', async ({ page }) => {
  const account = newAccount()

  await register(page, account)
  await expect(page.getByRole('heading', { name: 'Inbox' })).toBeVisible()

  // A brand new account has not confirmed its address yet.
  await expect(page.getByText(account.email)).toBeVisible()

  await page.click('a[href="/sent"]')
  await expect(page).toHaveURL(/\/sent$/)
  await expect(page.getByRole('heading', { name: 'Sent' })).toBeVisible()

  await page.click('button[aria-label="Account menu"]')
  await page.getByRole('menuitem', { name: 'Sign out' }).click()
  await page.waitForURL('**/login**')

  await page.fill('#email', account.email)
  await page.fill('#password', account.password)
  await page.click('button[type=submit]')
  await page.waitForURL('**/inbox')
  await expect(page.getByRole('heading', { name: 'Inbox' })).toBeVisible()
})

test('an already signed-in visitor is redirected away from sign in', async ({ page }) => {
  await register(page, newAccount())

  await page.goto('/login')
  await expect(page).toHaveURL(/\/inbox$/)
})

test('a wrong password is reported without leaking whether the account exists', async ({ page }) => {
  const account = newAccount()
  await register(page, account)

  await page.click('button[aria-label="Account menu"]')
  await page.getByRole('menuitem', { name: 'Sign out' }).click()
  await page.waitForURL('**/login**')

  await page.fill('#email', account.email)
  await page.fill('#password', 'definitely-the-wrong-password')
  await page.click('button[type=submit]')

  const error = page.getByRole('alert')
  await expect(error).toContainText('Email or password is incorrect.')

  // An unknown address must produce the very same message.
  await page.fill('#email', 'nobody-at-all@example.com')
  await page.fill('#password', 'definitely-the-wrong-password')
  await page.click('button[type=submit]')
  await expect(error).toContainText('Email or password is incorrect.')
})

test('registration rejects a password below the minimum length', async ({ page }) => {
  await page.goto('/register')
  await page.fill('#name', 'Too Short')
  await page.fill('#email', `short-${Date.now()}@example.com`)
  await page.fill('#password', 'short')
  await page.click('button[type=submit]')

  await expect(page.getByText('Must be at least 10 characters.')).toBeVisible()
  await expect(page).toHaveURL(/\/register$/)
})
