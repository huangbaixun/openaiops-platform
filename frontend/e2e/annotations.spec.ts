/**
 * PLATFORM-ASK-2 — Playwright E2E for AI annotation badges.
 *
 * Asserts:
 *  1. An ai_rca annotation seeded via POST /api/v1/annotations surfaces as the
 *     <AnnotationBadge> (data-testid="annotation-badge") on /services/checkout.
 *  2. A cross-tenant write (body tenant_id != caller's tenant) is rejected 403.
 *
 * Relative paths + config baseURL only — never hardcode a port. Mirrors the
 * other e2e specs (form login via apiKey-input).
 */

import { test, expect, type Page } from '@playwright/test'

async function loginAs(page: Page, key: string) {
  await page.goto('/login')
  await page.getByTestId('apiKey-input').locator('input').fill(key)
  await page.getByTestId('submit-btn').click()
  await expect(page).toHaveURL(/\/$/)
}

test('AI annotation badge appears on service detail', async ({ page, request }) => {
  const res = await request.post('/api/v1/annotations', {
    headers: { Authorization: 'Bearer test-key-acme', 'Content-Type': 'application/json' },
    data: {
      target_type: 'service', target_id: 'checkout', kind: 'ai_rca',
      payload: { summary: 'e2e seeded' }, ts: new Date().toISOString(),
    },
  })
  expect([200, 201]).toContain(res.status())

  await loginAs(page, 'test-key-acme')
  await page.goto('/services/checkout?window=1h')
  await expect(page.getByTestId('annotation-badge')).toBeVisible({ timeout: 10_000 })
})

test('cross-tenant write is blocked (403)', async ({ request }) => {
  const res = await request.post('/api/v1/annotations', {
    headers: { Authorization: 'Bearer test-key-acme', 'Content-Type': 'application/json' },
    data: {
      tenant_id: '22222222-2222-2222-2222-222222222222',
      target_type: 'service', target_id: 'checkout', kind: 'ai_rca',
      payload: {}, ts: new Date().toISOString(),
    },
  })
  expect(res.status()).toBe(403)
})

test('AI annotation badge appears on trace detail', async ({ page, request }) => {
  const listRes = await request.get('/api/v1/traces?window=1h', {
    headers: { Authorization: 'Bearer test-key-acme' },
  })
  expect(listRes.ok()).toBeTruthy()
  const data = await listRes.json()
  // TraceListResponse shape: { items: TraceListItem[], has_more: boolean }
  const traceId = data.items?.[0]?.trace_id ?? data[0]?.trace_id
  expect(traceId, 'a seeded trace must exist (run seed-traces)').toBeTruthy()

  const res = await request.post('/api/v1/annotations', {
    headers: { Authorization: 'Bearer test-key-acme', 'Content-Type': 'application/json' },
    data: { target_type: 'trace', target_id: traceId, kind: 'ai_rca', payload: { summary: 'e2e trace' }, ts: new Date().toISOString() },
  })
  expect([200, 201]).toContain(res.status())

  await loginAs(page, 'test-key-acme')
  await page.goto(`/traces/${traceId}`)
  await expect(page.getByTestId('annotation-badge')).toBeVisible({ timeout: 10_000 })
})

test('AI annotation marker appears on a topology node', async ({ page, request }) => {
  const res = await request.post('/api/v1/annotations', {
    headers: { Authorization: 'Bearer test-key-acme', 'Content-Type': 'application/json' },
    data: { target_type: 'service', target_id: 'checkout', kind: 'ai_rca', payload: { summary: 'e2e topo' }, ts: new Date().toISOString() },
  })
  expect([200, 201]).toContain(res.status())

  await loginAs(page, 'test-key-acme')
  await page.goto('/topology?window=1h')
  await expect(page.getByTestId('graph-node-ann-checkout')).toBeVisible({ timeout: 10_000 })
})
