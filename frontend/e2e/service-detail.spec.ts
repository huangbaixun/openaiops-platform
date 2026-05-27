/**
 * SLICE-3 T15 — Playwright E2E for /services/:name.
 *
 * Asserts:
 *  1. /services/<unknown> renders the not-found warning (404 handled gracefully).
 *  2. /services/checkout (if seeded by make seed-topology) renders NTabs OR
 *     the not-found warning — both prove the page mounted past auth.
 *  3. TimeWindowPicker click reflects in URL.
 *  4. beta tenant cannot see /services/<acme-service>.
 */

import { test, expect, type Page } from '@playwright/test'

async function loginAs(page: Page, key: string) {
  await page.goto('/login')
  await page.getByTestId('apiKey-input').locator('input').fill(key)
  await page.getByTestId('submit-btn').click()
  await expect(page).toHaveURL(/\/$/)
}

const NOT_FOUND_TEXT = /Service has no data|该服务在当前时间窗内无数据/

test('acme: /services/<unknown> shows not-found warning (404 handled gracefully)', async ({ page }) => {
  await loginAs(page, 'test-key-acme')
  await page.goto('/services/this-service-does-not-exist?window=1h')
  // i18n locale-agnostic text match — ServiceDetail.vue renders <NAlert type=warning>
  // with services.notFound when fetchServiceDetail returns null.
  await expect(page.getByText(NOT_FOUND_TEXT)).toBeVisible({ timeout: 10_000 })
})

test('acme: /services/checkout mounts (NTabs OR not-found — both prove past-auth render)', async ({ page }) => {
  await loginAs(page, 'test-key-acme')
  await page.goto('/services/checkout?window=1h')
  // Either NTabs renders (topo-engine populated checkout) OR not-found shows.
  // Both are valid — auth + route worked. Tabs span CN/EN text.
  await expect(page.getByTestId('time-window-picker')).toBeVisible({ timeout: 10_000 })
  const tabsOrNotFound = page.locator('.n-tabs').or(page.getByText(NOT_FOUND_TEXT))
  await expect(tabsOrNotFound.first()).toBeVisible({ timeout: 10_000 })
})

test('acme: TimeWindowPicker click reflects in URL on service detail', async ({ page }) => {
  await loginAs(page, 'test-key-acme')
  await page.goto('/services/checkout?window=1h')
  await expect(page.getByTestId('time-window-picker')).toBeVisible({ timeout: 10_000 })
  await page.getByTestId('window-15m').click()
  await expect(page).toHaveURL(/window=15m/, { timeout: 5_000 })
})

test('beta: /services/checkout shows not-found (cross-tenant)', async ({ browser }) => {
  const ctx = await browser.newContext({ ignoreHTTPSErrors: true })
  const page = await ctx.newPage()
  await loginAs(page, 'test-key-beta')
  await page.goto('/services/checkout?window=1h')
  await expect(page.getByText(NOT_FOUND_TEXT)).toBeVisible({ timeout: 10_000 })
  await ctx.close()
})
