import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuthStore } from '../auth'

vi.mock('../../api/client', () => ({
  default: { get: vi.fn(), post: vi.fn() },
}))
vi.mock('../../api/tenants', () => ({
  fetchTenants: vi.fn().mockResolvedValue([
    { id: 'home', name: 'shop-prod', environment: 'prod' },
    { id: 'peer', name: 'shop-staging', environment: 'staging' },
  ]),
  switchTenant: vi.fn().mockResolvedValue(undefined),
}))

describe('auth store — tenant switching', () => {
  beforeEach(() => { setActivePinia(createPinia()); localStorage.clear() })

  it('switchActiveTenant sets active id + persists', async () => {
    const auth = useAuthStore()
    auth.tenantId = 'home'
    await auth.switchActiveTenant('peer')
    expect(auth.activeTenantId).toBe('peer')
    expect(localStorage.getItem('activeTenantId')).toBe('peer')
  })

  it('logout clears the active tenant', () => {
    const auth = useAuthStore()
    localStorage.setItem('activeTenantId', 'peer')
    auth.activeTenantId = 'peer'
    auth.logout()
    expect(auth.activeTenantId).toBeNull()
    expect(localStorage.getItem('activeTenantId')).toBeNull()
  })

  it('loadTenants populates domainTenants + defaults active to home when no valid persisted', async () => {
    const auth = useAuthStore()
    auth.tenantId = 'home'
    auth.tenantName = 'shop-prod'
    await auth.loadTenants()
    expect(auth.domainTenants.length).toBe(2)
    expect(auth.activeTenantId).toBe('home')
  })
})
