/**
 * SLICE-2 T14 — Playwright E2E for /logs + Logs subtab + drift D4 regression.
 *
 * Prerequisites (stack must be up + seeded):
 *   make up && make seed-traces && make seed-logs
 *   (seed-traces needs INGESTER_OTLP_GRPC_HOST_PORT=14317 on this machine due to .env override)
 *
 * 5 test cases:
 *  1. Lists seeded logs — /logs renders ≥5 rows including FATAL severity
 *  2. trace_id chip cross-jumps to /traces/:id
 *  3. Logs subtab on /traces/:id shows trace-scoped logs
 *  4. Drift D4 regression — direct frontend host port (13000) returns 404 for /api/v1/logs
 *  5. Cross-tenant — beta key sees "暂无日志" empty state (no log leakage)
 */

import { test, expect, type Page } from '@playwright/test'

// baseURL from playwright.config.ts → https://localhost (Caddy ingress)
const FRONTEND_HOST_PORT = process.env.FRONTEND_HOST_PORT ?? '13000'

/** Shared login helper — fills the NaiveUI NInput wrapper and submits. */
async function loginAs(page: Page, key: string) {
  await page.goto('/login')
  // NaiveUI NInput puts data-testid on the wrapper div; drill into the real <input>.
  await page.getByTestId('apiKey-input').locator('input').fill(key)
  await page.getByTestId('submit-btn').click()
  await expect(page).toHaveURL(/\/$/)
}

// ---------------------------------------------------------------------------
// Test 1: Lists seeded logs — renders ≥5 rows, FATAL badge visible
// ---------------------------------------------------------------------------
test('acme: /logs renders seeded rows including FATAL severity', async ({ page }) => {
  await loginAs(page, 'test-key-acme')
  await page.getByTestId('nav-logs').click()
  await expect(page).toHaveURL(/\/logs/)

  // Wait for list container to appear (logs async-load on mount)
  const logsList = page.getByTestId('logs-list')
  await expect(logsList).toBeVisible({ timeout: 10_000 })

  // Seed emits ≥5 records (seed may run more than once; assert lower bound).
  const rows = logsList.locator('.log-row')
  const count = await rows.count()
  expect(count).toBeGreaterThanOrEqual(5)

  // At least one FATAL badge must be visible in the list
  await expect(logsList.getByText('FATAL').first()).toBeVisible()
})

// ---------------------------------------------------------------------------
// Test 2: trace_id chip cross-jumps to /traces/:id
// ---------------------------------------------------------------------------
test('acme: clicking trace_id chip in log row navigates to /traces/:id', async ({ page }) => {
  await loginAs(page, 'test-key-acme')
  await page.getByTestId('nav-logs').click()

  const logsList = page.getByTestId('logs-list')
  await expect(logsList).toBeVisible({ timeout: 10_000 })

  // All seeded log rows carry a trace_id chip (trace_id = 4bf92f3577b34da6a3ce929d0e0e4736)
  const traceChip = logsList.locator('.trace-chip').first()
  await expect(traceChip).toBeVisible({ timeout: 10_000 })

  await traceChip.click()

  // Should land on /traces/<hex-id>
  await expect(page).toHaveURL(/\/traces\/[0-9a-f]+/, { timeout: 10_000 })
})

// ---------------------------------------------------------------------------
// Test 3: Logs subtab on /traces/:id shows trace-scoped logs
// ---------------------------------------------------------------------------
test('acme: Logs subtab on /traces/:id shows scoped log rows', async ({ page }) => {
  await loginAs(page, 'test-key-acme')

  // Navigate to traces list to find a trace with logs
  // seed-logs pins trace_id=4bf92f3577b34da6a3ce929d0e0e4736; navigate directly
  await page.goto('/traces/4bf92f3577b34da6a3ce929d0e0e4736')
  // Page might redirect to login if session not set — we do proper login first above,
  // but since goto replaces the history, re-login guard may fire. Use full flow:
  // (goto after login should keep auth store; Vue Router guard checks localStorage)
  await expect(page).toHaveURL(/\/traces\/4bf92f3577b34da6a3ce929d0e0e4736/)

  // Click the 日志 / Logs tab
  await page.locator('.n-tabs-tab', { hasText: /日志|Logs/ }).click()

  // LogsPanel renders log rows via logs-panel-list data-testid
  const logsPanel = page.getByTestId('logs-panel-list')
  await expect(logsPanel).toBeVisible({ timeout: 10_000 })
  await expect(logsPanel.locator('.log-row').first()).toBeVisible()
})

// ---------------------------------------------------------------------------
// Test 4: Drift D4 regression — direct frontend host port does NOT proxy /api
// ---------------------------------------------------------------------------
test('D4 regression: http://localhost:<frontend-port>/api/v1/logs does NOT proxy to backend', async ({ request }) => {
  // The frontend nginx serves only static files; /api/* is NOT proxied.
  // nginx falls back to try_files → index.html (200 SPA HTML) for any unmatched path.
  // The architectural assertion is that the response body is NOT a backend JSON response
  // (i.e., no {"items":[...]} from the query service) — which would indicate an /api
  // proxy block had been re-added. nginx SPA fallback returns HTML, not JSON.
  const resp = await request.get(`http://localhost:${FRONTEND_HOST_PORT}/api/v1/logs`, {
    headers: { Authorization: 'Bearer test-key-acme' },
    // Playwright APIRequestContext does not follow HTML vs JSON content negotiation.
  })
  // nginx SPA fallback: 200 + HTML body. Backend proxy would return 200 + JSON.
  // Distinguish by checking Content-Type — nginx returns text/html, not application/json.
  const contentType = resp.headers()['content-type'] ?? ''
  expect(contentType).toMatch(/text\/html/)
  // Belt-and-suspenders: body should not contain the "items" key from the log API shape.
  const body = await resp.text()
  expect(body).not.toMatch(/"items"/)
})

// ---------------------------------------------------------------------------
// Test 5: Cross-tenant — beta key sees empty state, no acme logs leak
// ---------------------------------------------------------------------------
test('beta: /logs shows empty state (cross-tenant isolation)', async ({ browser }) => {
  // Use a fresh browser context so beta auth does not share acme localStorage.
  const ctx = await browser.newContext({ ignoreHTTPSErrors: true })
  const page = await ctx.newPage()
  await loginAs(page, 'test-key-beta')
  await page.getByTestId('nav-logs').click()
  await expect(page).toHaveURL(/\/logs/)

  // Logs list must NOT appear; empty state must be visible.
  await expect(page.getByTestId('logs-list')).not.toBeVisible({ timeout: 8_000 })
  await expect(page.getByTestId('logs-empty')).toBeVisible({ timeout: 8_000 })
  // Confirm the empty copy text (zh-CN: 暂无日志 / en-US: "No logs…")
  await expect(page.getByTestId('logs-empty')).not.toBeEmpty()

  await ctx.close()
})
