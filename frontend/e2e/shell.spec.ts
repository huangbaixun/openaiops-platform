import { test, expect, type Page } from '@playwright/test'

async function login(page: Page) {
  await page.goto('/login')
  await page.getByTestId('apiKey-input').locator('input').fill('test-key-acme')
  await page.getByTestId('submit-btn').click()
  await expect(page).toHaveURL(/\/$/)
}

test('⌘K palette opens and jumps to a service', async ({ page }) => {
  await login(page)
  await page.goto('/overview')
  // Wait for AppLayout to mount so the window keydown listener is registered
  await expect(page.locator('.app')).toBeVisible()
  await page.keyboard.press('Control+k')
  await expect(page.getByTestId('command-palette')).toBeVisible()
  const cmdkInput = page.getByTestId('cmdk-input')
  await cmdkInput.fill('checkout')
  // Wait for the checkout result to appear before pressing Enter
  await expect(page.locator('[data-testid="cmdk-item-checkout"]')).toBeVisible()
  await cmdkInput.press('Enter')
  await expect(page).toHaveURL(/\/services\/checkout/)
})

test('theme toggle persists across reload', async ({ page }) => {
  await login(page)
  await page.getByTestId('theme-toggle').click()
  const theme = await page.evaluate(() => document.documentElement.getAttribute('data-theme'))
  expect(theme).toBe('dark')
  await page.reload()
  // Wait for AppLayout to mount so initTheme() has run and set data-theme
  await expect(page.locator('.topbar')).toBeVisible()
  const after = await page.evaluate(() => document.documentElement.getAttribute('data-theme'))
  expect(after).toBe('dark')
})

test('sidebar collapse persists across reload', async ({ page }) => {
  await login(page)
  await page.locator('.sidebar-toggle').click()
  await expect(page.locator('.app.sidebar-collapsed')).toBeVisible()
  await page.reload()
  await expect(page.locator('.app.sidebar-collapsed')).toBeVisible()
})
