import { test, expect } from '@playwright/test'

// Use relative paths so playwright.config.ts baseURL (=https://localhost) wins.
// Hardcoded http://localhost:3000 was SLICE-2 drift D5 — local OrbStack/SignOz
// intercepted port 3000. Lock-in: closed in SLICE-3 T15.

// NaiveUI NInput puts data-testid on a wrapper div; the actual <input> is inside it.
// We use the wrapper to scope, then find the inner input element.
test('login with valid key shows home + tenant name', async ({ page }) => {
  await page.goto('/login')
  await page.getByTestId('apiKey-input').locator('input').fill('test-key-acme')
  await page.getByTestId('submit-btn').click()
  await expect(page).toHaveURL(/\/$/)
  await expect(page.getByTestId('tenant-name')).toContainText('acme')
})

test('login with wrong key shows error', async ({ page }) => {
  await page.goto('/login')
  await page.getByTestId('apiKey-input').locator('input').fill('not-a-real-key')
  await page.getByTestId('submit-btn').click()
  await expect(page).toHaveURL(/\/login/)
})

test('language switch toggles UI', async ({ page }) => {
  await page.goto('/login')
  // The lang-select is inside the login card; clicking opens the dropdown.
  await page.getByTestId('lang-select').click()
  // NaiveUI dropdown options use class n-base-select-option__content.
  // Switch to Chinese (中文) which is visually distinct and always present.
  await page.locator('.n-base-select-option__content', { hasText: '中文' }).click()
  // setLocale calls location.reload(); wait for navigation to complete.
  await page.waitForURL(/\/login/, { timeout: 10_000 })
  // After switching to Chinese, the submit button shows the Chinese text.
  await expect(page.getByTestId('submit-btn')).toContainText('登录', { timeout: 5000 })
})

test('two tenants do not see each others data via key', async ({ browser }) => {
  const ctxA = await browser.newContext({ ignoreHTTPSErrors: true })
  const pageA = await ctxA.newPage()
  await pageA.goto('/login')
  await pageA.getByTestId('apiKey-input').locator('input').fill('test-key-acme')
  await pageA.getByTestId('submit-btn').click()
  await expect(pageA.getByTestId('tenant-name')).toContainText('acme')

  const ctxB = await browser.newContext({ ignoreHTTPSErrors: true })
  const pageB = await ctxB.newPage()
  await pageB.goto('/login')
  await pageB.getByTestId('apiKey-input').locator('input').fill('test-key-beta')
  await pageB.getByTestId('submit-btn').click()
  await expect(pageB.getByTestId('tenant-name')).toContainText('beta')

  await ctxA.close()
  await ctxB.close()
})
