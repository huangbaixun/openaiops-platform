/**
 * SLICE-3 T15 — Playwright E2E for /topology.
 *
 * Prerequisites (stack up):
 *   make up && (optional) make seed-topology
 *
 * Asserts:
 *  1. /topology page mounts (TimeWindowPicker visible — proves no auth/load error).
 *  2. TimeWindowPicker updates the URL ?window= parameter.
 *  3. SideBar nav-topology entry navigates to /topology.
 *  4. beta tenant sees zero acme graph nodes (cross-tenant isolation).
 *
 * Note: data-bearing assertions are owned by the Go integration test
 * TestSlice3_CrossTenantTopology + the topoengine RunOnce_WritesEdges suite.
 * Playwright proves the FRONTEND wires up correctly through Caddy + auth.
 */

import { test, expect, type Page } from '@playwright/test'

async function loginAs(page: Page, key: string) {
  await page.goto('/login')
  await page.getByTestId('apiKey-input').locator('input').fill(key)
  await page.getByTestId('submit-btn').click()
  await expect(page).toHaveURL(/\/$/)
}

test('acme: /topology page mounts (TimeWindowPicker proves no auth/load error)', async ({ page }) => {
  await loginAs(page, 'test-key-acme')
  await page.goto('/topology?window=1h')
  await expect(page.getByTestId('time-window-picker')).toBeVisible({ timeout: 10_000 })
  // Either the d3-force SVG renders OR <NEmpty graph-empty> placeholder shows.
  // Both are valid; only an axios error would render <NAlert type=error>.
  await expect(
    page.locator('[data-testid="service-graph"], [data-testid="graph-empty"]')
  ).toBeVisible({ timeout: 10_000 })
})

test('acme: TimeWindowPicker click reflects in URL', async ({ page }) => {
  await loginAs(page, 'test-key-acme')
  await page.goto('/topology?window=1h')
  await expect(page.getByTestId('time-window-picker')).toBeVisible({ timeout: 10_000 })
  await page.getByTestId('window-6h').click()
  await expect(page).toHaveURL(/window=6h/, { timeout: 5_000 })
})

test('acme: SideBar nav-topology link goes to /topology', async ({ page }) => {
  await loginAs(page, 'test-key-acme')
  await page.getByTestId('nav-topology').click()
  await expect(page).toHaveURL(/\/topology/)
})

test('beta: /topology renders without leaking acme nodes', async ({ browser }) => {
  const ctx = await browser.newContext({ ignoreHTTPSErrors: true })
  const page = await ctx.newPage()
  await loginAs(page, 'test-key-beta')
  await page.goto('/topology?window=1h')
  await expect(page.getByTestId('time-window-picker')).toBeVisible({ timeout: 10_000 })
  // beta MUST NOT render acme service nodes regardless of whether the empty
  // placeholder or an SVG appears.
  await expect(page.locator('[data-testid="graph-node-checkout"]')).toHaveCount(0)
  await expect(page.locator('[data-testid="graph-node-frontend"]')).toHaveCount(0)
  await ctx.close()
})
