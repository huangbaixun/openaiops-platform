import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import TopBar from '../TopBar.vue'
import { useAuthStore } from '../../stores/auth'

const push = vi.fn()
vi.mock('vue-router', () => ({
  useRouter: () => ({ push }),
  useRoute: () => ({ query: {} }),
}))
const i18n = createI18n({ legacy: false, locale: 'en-US', messages: { 'en-US': {
  shell: { logout: 'Logout' }, topbar: { tenant: 'Tenant' },
} } })

describe('TopBar', () => {
  beforeEach(() => { setActivePinia(createPinia()); push.mockClear() })

  it('renders brand, scope-pill, theme toggle, and avatar', () => {
    const w = mount(TopBar, { global: { plugins: [i18n], stubs: { NSelect: true } } })
    expect(w.find('.brand').exists()).toBe(true)
    expect(w.find('[data-testid="scope-project"]').exists()).toBe(true)
    expect(w.find('[data-testid="theme-toggle"]').exists()).toBe(true)
    expect(w.find('[data-testid="user-avatar"]').exists()).toBe(true)
  })

  it('logout clears auth and routes to login', async () => {
    const auth = useAuthStore()
    auth.tenantName = 'acme'
    const logoutSpy = vi.spyOn(auth, 'logout')
    const w = mount(TopBar, { global: { plugins: [i18n], stubs: { NSelect: true } } })
    await w.get('[data-testid="logout-btn"]').trigger('click')
    expect(logoutSpy).toHaveBeenCalled()
    expect(push).toHaveBeenCalledWith({ name: 'login' })
  })
})
