import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import ScopePill from '../ScopePill.vue'
import { useAuthStore } from '../../stores/auth'

const i18n = createI18n({ legacy: false, locale: 'en-US', messages: { 'en-US': {} } })

describe('ScopePill', () => {
  beforeEach(() => { setActivePinia(createPinia()); localStorage.clear() })

  it('shows the current active tenant as the Project segment', () => {
    const auth = useAuthStore()
    auth.tenantName = 'acme'
    auth.tenantId = 'home'
    auth.activeTenantId = 'home'
    auth.domainTenants = [{ id: 'home', name: 'acme', environment: 'prod' }]
    const w = mount(ScopePill, { global: { plugins: [i18n] } })
    expect(w.get('[data-testid="scope-project"]').text()).toContain('acme')
  })

  it('renders Domain and Env segments', () => {
    const w = mount(ScopePill, { global: { plugins: [i18n] } })
    expect(w.find('[data-testid="scope-domain"]').exists()).toBe(true)
    expect(w.find('[data-testid="scope-env"]').exists()).toBe(true)
  })

  it('clicking a domain peer calls switchActiveTenant', async () => {
    const auth = useAuthStore()
    auth.tenantId = 'home'
    auth.tenantName = 'shop-prod'
    auth.activeTenantId = 'home'
    auth.domainTenants = [
      { id: 'home', name: 'shop-prod', environment: 'prod' },
      { id: 'peer', name: 'shop-staging', environment: 'staging' },
    ]
    const spy = vi.spyOn(auth, 'switchActiveTenant').mockResolvedValue()
    const w = mount(ScopePill, { global: { plugins: [i18n] } })
    await w.get('[data-testid="scope-project"]').trigger('click')
    await w.get('[data-testid="tenant-opt-peer"]').trigger('click')
    await flushPromises()
    expect(spy).toHaveBeenCalledWith('peer')
  })
})
