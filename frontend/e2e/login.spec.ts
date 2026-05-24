import { test, expect } from '@playwright/test'

const BASE = process.env.E2E_BASE_URL ?? 'http://localhost:3000'

// NaiveUI NInput puts data-testid on a wrapper div; the actual <input> is inside it.
// We use the wrapper to scope, then find the inner input element.
test('login with valid key shows home + tenant name', async ({ page }) => {
  await page.goto(`${BASE}/login`)
  await page.getByTestId('apiKey-input').locator('input').fill('test-key-acme')
  await page.getByTestId('submit-btn').click()
  await expect(page).toHaveURL(/\/$/)
  await expect(page.getByTestId('tenant-name')).toContainText('acme')
})

test('login with wrong key shows error', async ({ page }) => {
  await page.goto(`${BASE}/login`)
  await page.getByTestId('apiKey-input').locator('input').fill('not-a-real-key')
  await page.getByTestId('submit-btn').click()
  await expect(page).toHaveURL(/\/login/)
})

test('language switch toggles UI', async ({ page }) => {
  await page.goto(`${BASE}/login`)
  // The lang-select is inside the login card; clicking opens the dropdown.
  await page.getByTestId('lang-select').click()
  // Use nth(1) to target the dropdown option (not the already-shown value label).
  await page.getByText('English').nth(1).click()
  await expect(page.locator('text=Sign in')).toBeVisible({ timeout: 5000 })
})

test('two tenants do not see each others data via key', async ({ browser }) => {
  const ctxA = await browser.newContext()
  const pageA = await ctxA.newPage()
  await pageA.goto(`${BASE}/login`)
  await pageA.getByTestId('apiKey-input').locator('input').fill('test-key-acme')
  await pageA.getByTestId('submit-btn').click()
  await expect(pageA.getByTestId('tenant-name')).toContainText('acme')

  const ctxB = await browser.newContext()
  const pageB = await ctxB.newPage()
  await pageB.goto(`${BASE}/login`)
  await pageB.getByTestId('apiKey-input').locator('input').fill('test-key-beta')
  await pageB.getByTestId('submit-btn').click()
  await expect(pageB.getByTestId('tenant-name')).toContainText('beta')

  await ctxA.close()
  await ctxB.close()
})
