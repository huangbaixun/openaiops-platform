import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuthStore } from '../../src/stores/auth'

vi.mock('../../src/api/client', () => ({
  default: {
    get: vi.fn(async (url: string) => {
      if (url === '/healthz') {
        return { data: { status: 'ok', tenant_id: 'tid-1', tenant_name: 'acme' } }
      }
      throw new Error('unexpected')
    }),
  },
}))

describe('useAuthStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
  })

  it('login persists key + resolves tenant', async () => {
    const store = useAuthStore()
    await store.login('test-key-acme')
    expect(store.isAuthenticated).toBe(true)
    expect(store.tenantId).toBe('tid-1')
    expect(store.tenantName).toBe('acme')
    expect(localStorage.getItem('apiKey')).toBe('test-key-acme')
  })

  it('logout clears state + storage', async () => {
    const store = useAuthStore()
    await store.login('test-key-acme')
    store.logout()
    expect(store.isAuthenticated).toBe(false)
    expect(store.tenantId).toBe(null)
    expect(localStorage.getItem('apiKey')).toBe(null)
  })
})
