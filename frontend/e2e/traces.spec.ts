import { test, expect, type Page } from '@playwright/test'

// Use playwright.config.ts baseURL (https://localhost via Caddy). Relative paths
// are resolved against baseURL automatically; no need for a local BASE constant.

async function loginAs(page: Page, key: string) {
  await page.goto('/login')
  await page.getByTestId('apiKey-input').locator('input').fill(key)
  await page.getByTestId('submit-btn').click()
  await expect(page).toHaveURL(/\/$/)
}

test('acme: traces list renders + clicking row goes to detail with waterfall', async ({ page }) => {
  await loginAs(page, 'test-key-acme')
  await page.getByTestId('nav-traces').click()
  await expect(page).toHaveURL(/\/traces$/)
  await expect(page.getByTestId('traces-table')).toBeVisible()

  // T15 seeded 3 traces × 5 spans. Wait until at least one row appears.
  const firstRow = page.locator('[data-testid=traces-table] tbody tr').first()
  await expect(firstRow).toBeVisible({ timeout: 10_000 })
  await firstRow.click()

  await expect(page).toHaveURL(/\/traces\/[0-9a-f]+/)
  await expect(page.getByTestId('waterfall-svg')).toBeVisible()
  // Seeded traces have 5 spans each; assert at least 1 span is rendered.
  const spanCount = await page.locator('[data-testid=waterfall-span]').count()
  expect(spanCount).toBeGreaterThanOrEqual(5)
})

test('JSON tab shows raw payload', async ({ page }) => {
  await loginAs(page, 'test-key-acme')
  await page.getByTestId('nav-traces').click()
  const firstRow = page.locator('[data-testid=traces-table] tbody tr').first()
  await expect(firstRow).toBeVisible({ timeout: 10_000 })
  await firstRow.click()

  // NaiveUI NTabs renders tabs as generic clickable divs (no role=tab).
  // Click the tab label inside the tabs nav.
  await page.locator('.n-tabs-tab', { hasText: 'JSON' }).click()
  await expect(page.getByTestId('trace-json')).toBeVisible()
})

test('Service Map tab shows ServiceGraph (replaced SLICE-2 placeholder in SLICE-3 T14)', async ({ page }) => {
  await loginAs(page, 'test-key-acme')
  await page.getByTestId('nav-traces').click()
  const firstRow = page.locator('[data-testid=traces-table] tbody tr').first()
  await expect(firstRow).toBeVisible({ timeout: 10_000 })
  await firstRow.click()

  await page.locator('.n-tabs-tab', { hasText: /(Service Map|服务地图)/ }).click()
  // SLICE-3 T14 replaced the static placeholder with the real ServiceMapPanel
  // which derives nodes+edges client-side from already-loaded spans (zero
  // backend call). Either the SVG renders OR <NEmpty graph-empty> shows if
  // the trace has no parent_span_id chains — both prove the panel mounted.
  await expect(
    page.locator('[data-testid="service-graph"], [data-testid="graph-empty"]')
  ).toBeVisible({ timeout: 5_000 })
})

test('beta: empty list (cross-tenant UX, no leakage)', async ({ browser }) => {
  const ctx = await browser.newContext()
  const page = await ctx.newPage()
  await loginAs(page, 'test-key-beta')
  await page.getByTestId('nav-traces').click()
  // No rows; "empty" copy visible.
  await expect(page.locator('[data-testid=traces-table] tbody tr')).toHaveCount(0)
  await expect(page.getByText(/No traces|当前时间窗内无调用链/)).toBeVisible()
  await ctx.close()
})
