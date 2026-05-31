import { test, expect, type Page } from '@playwright/test'

async function login(page: Page, key: string) {
  await page.goto('/login')
  await page.getByTestId('apiKey-input').locator('input').fill(key)
  await page.getByTestId('submit-btn').click()
  await expect(page).toHaveURL(/\/$/)
}

test('domain key lists peers and switches active tenant', async ({ page }) => {
  await login(page, 'test-key-domain')
  await page.getByTestId('scope-project').click()
  await expect(page.getByTestId('tenant-opt-44444444-4444-4444-4444-444444444444')).toBeVisible()
  await page.getByTestId('tenant-opt-44444444-4444-4444-4444-444444444444').click()
  await expect(page.getByTestId('scope-env')).toContainText('staging')
})

test('read-write key shows a single project (no peers)', async ({ page }) => {
  await login(page, 'test-key-acme')
  await page.getByTestId('scope-project').click()
  await expect(page.getByTestId('scope-project')).toContainText('acme')
})

test('out-of-domain switch is blocked by the backend (403)', async ({ request }) => {
  const res = await request.post('/api/v1/tenants/switch', {
    headers: { Authorization: 'Bearer test-key-domain', 'Content-Type': 'application/json' },
    data: { tenant_id: '11111111-1111-1111-1111-111111111111' },
  })
  expect(res.status()).toBe(403)
})
