import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import ScopePill from '../ScopePill.vue'
import { useAuthStore } from '../../stores/auth'

const i18n = createI18n({ legacy: false, locale: 'en-US', messages: { 'en-US': {} } })

describe('ScopePill', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('shows the current tenant as the Project segment', () => {
    const auth = useAuthStore()
    auth.tenantName = 'acme'
    const w = mount(ScopePill, { global: { plugins: [i18n] } })
    expect(w.get('[data-testid="scope-project"]').text()).toContain('acme')
  })

  it('renders Domain and Env segments as read-only (no switching handlers)', () => {
    const w = mount(ScopePill, { global: { plugins: [i18n] } })
    expect(w.find('[data-testid="scope-domain"]').exists()).toBe(true)
    expect(w.find('[data-testid="scope-env"]').exists()).toBe(true)
  })
})
