/**
 * SLICE-3 T15 — Playwright E2E for /overview.
 *
 * Prerequisites: stack up; make seed-topology optional (page renders even with
 * zero data, since topo-engine population is gated by known drift D6 —
 * AdminConn Row Policy bypass needed in prod).
 *
 * Asserts:
 *  1. /overview page mounts (TimeWindowPicker visible — proves no auth/route error).
 *  2. SideBar nav-overview navigates to /overview.
 *  3. TimeWindowPicker click updates URL.
 *  4. Clicking a service card navigates to /services/:name (skipped if no cards).
 *  5. beta tenant sees zero acme cards (cross-tenant isolation regression).
 */

import { test, expect, type Page } from '@playwright/test'

async function loginAs(page: Page, key: string) {
  await page.goto('/login')
  await page.getByTestId('apiKey-input').locator('input').fill(key)
  await page.getByTestId('submit-btn').click()
  await expect(page).toHaveURL(/\/$/)
}

test('acme: /overview page mounts (TimeWindowPicker proves no auth/load error)', async ({ page }) => {
  await loginAs(page, 'test-key-acme')
  await page.goto('/overview?window=1h')
  // TimeWindowPicker renders only after the page-level load finishes (not gated on data);
  // its presence + no <NAlert type=error> proves auth+route reached the API successfully.
  await expect(page.getByTestId('time-window-picker')).toBeVisible({ timeout: 10_000 })
  // No error alert — if there was an axios failure (e.g., 401/500), <NAlert type=error> would render.
  await expect(page.locator('.n-alert')).toHaveCount(0)
})

test('acme: SideBar nav-overview navigates to /overview', async ({ page }) => {
  await loginAs(page, 'test-key-acme')
  await page.getByTestId('nav-overview').click()
  await expect(page).toHaveURL(/\/overview/)
  await expect(page.getByTestId('time-window-picker')).toBeVisible({ timeout: 10_000 })
})

test('acme: TimeWindowPicker click reflects in URL', async ({ page }) => {
  await loginAs(page, 'test-key-acme')
  await page.goto('/overview?window=1h')
  await expect(page.getByTestId('time-window-picker')).toBeVisible({ timeout: 10_000 })
  await page.getByTestId('window-24h').click()
  await expect(page).toHaveURL(/window=24h/, { timeout: 5_000 })
})

test('acme: clicking a service card navigates to /services/:name (if cards present)', async ({ page }) => {
  await loginAs(page, 'test-key-acme')
  await page.goto('/overview?window=1h')
  await expect(page.getByTestId('time-window-picker')).toBeVisible({ timeout: 10_000 })

  // Cards depend on topo-engine having populated service_stats_v1 (drift D6 in prod).
  const firstCard = page.locator('[data-testid^="service-card-"]').first()
  if ((await firstCard.count()) > 0) {
    await firstCard.click()
    await expect(page).toHaveURL(/\/services\/[^/]+/, { timeout: 5_000 })
  }
})

test('beta: /overview shows zero acme cards (cross-tenant isolation)', async ({ browser }) => {
  const ctx = await browser.newContext({ ignoreHTTPSErrors: true })
  const page = await ctx.newPage()
  await loginAs(page, 'test-key-beta')
  await page.goto('/overview?window=1h')
  await expect(page.getByTestId('time-window-picker')).toBeVisible({ timeout: 10_000 })
  // Belt-and-suspenders: no acme service cards should ever appear under beta.
  await expect(page.locator('[data-testid="service-card-checkout"]')).toHaveCount(0)
  await expect(page.locator('[data-testid="service-card-frontend"]')).toHaveCount(0)
  await ctx.close()
})
