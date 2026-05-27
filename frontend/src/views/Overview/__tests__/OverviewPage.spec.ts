import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createI18n } from 'vue-i18n'
import OverviewPage from '../OverviewPage.vue'

// Mock the fetchServices module.
vi.mock('../../../api/services', () => ({
  fetchServices: vi.fn(),
}))
import { fetchServices } from '../../../api/services'

describe('OverviewPage', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('renders one card per service after fetch', async () => {
    (fetchServices as any).mockResolvedValue({
      window: '1h',
      items: [
        { service: 'frontend', inbound_calls: 100, inbound_errors: 1, inbound_error_rate: 0.01, inbound_p95_ms: 12, outbound_calls: 50 },
        { service: 'checkout', inbound_calls: 50, inbound_errors: 0, inbound_error_rate: 0, inbound_p95_ms: 30, outbound_calls: 20 },
      ],
    })

    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/overview', component: OverviewPage },
        { path: '/services/:name', component: { template: '<div/>' } },
      ],
    })
    await router.push('/overview?window=1h')
    await router.isReady()

    const i18n = createI18n({
      locale: 'en', legacy: false,
      messages: { en: { overview: { title: 'Overview' }, timeWindow: { '15m': '15m', '1h': '1h', '6h': '6h', '24h': '24h' } } },
    })

    const w = mount(OverviewPage, { global: { plugins: [router, i18n] } })
    await flushPromises()
    expect(w.find('[data-testid="service-card-frontend"]').exists()).toBe(true)
    expect(w.find('[data-testid="service-card-checkout"]').exists()).toBe(true)
  })

  it('shows error alert on fetch failure', async () => {
    (fetchServices as any).mockRejectedValue(new Error('500'))

    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/overview', component: OverviewPage }],
    })
    await router.push('/overview')
    await router.isReady()

    const i18n = createI18n({
      locale: 'en', legacy: false,
      messages: { en: { overview: { title: 'Overview' }, timeWindow: { '15m': '15m', '1h': '1h', '6h': '6h', '24h': '24h' } } },
    })

    const w = mount(OverviewPage, { global: { plugins: [router, i18n] } })
    await flushPromises()
    expect(w.text()).toContain('500')
  })
})
