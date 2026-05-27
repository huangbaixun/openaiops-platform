import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createI18n } from 'vue-i18n'
import ServiceDetail from '../ServiceDetail.vue'

vi.mock('../../../api/services', () => ({
  fetchServiceDetail: vi.fn(),
}))
import { fetchServiceDetail } from '../../../api/services'

function makeI18n() {
  return createI18n({
    locale: 'en', legacy: false,
    messages: {
      en: {
        services: {
          notFound: 'Service not found',
          tabSignals: 'Signals',
          tabDependencies: 'Dependencies',
          tabRuntime: 'Runtime',
          tabExceptions: 'Exceptions',
          tabAlerts: 'Alerts',
          tabSettings: 'Settings',
          comingSoon: 'Coming in {slice}',
          calls: 'Calls', errors: 'Errors',
          viewTraces: 'View traces', viewLogs: 'View logs',
          depInbound: 'Inbound', depOutbound: 'Outbound',
        },
        timeWindow: { '15m': '15m', '1h': '1h', '6h': '6h', '24h': '24h' },
        topology: { empty: 'No data' },
      },
    },
  })
}

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/services/:name', component: ServiceDetail },
      { path: '/traces', component: { template: '<div/>' } },
      { path: '/logs', component: { template: '<div/>' } },
    ],
  })
}

describe('ServiceDetail', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('renders all six tabs on happy path', async () => {
    (fetchServiceDetail as any).mockResolvedValue({
      service: 'frontend',
      window: '1h',
      stats: {
        inbound: { calls: 100, errors: 1, error_rate: 0.01, p95_ms: 12.3 },
        outbound: { calls: 50, errors: 0, error_rate: 0, p95_ms: 8.0 },
      },
      dependencies: { inbound: [], outbound: [] },
    })
    const router = makeRouter()
    await router.push('/services/frontend?window=1h')
    await router.isReady()
    const w = mount(ServiceDetail, { global: { plugins: [router, makeI18n()] } })
    await flushPromises()
    const text = w.text()
    expect(text).toContain('Signals')
    expect(text).toContain('Dependencies')
    expect(text).toContain('Runtime')
    expect(text).toContain('Exceptions')
    expect(text).toContain('Alerts')
    expect(text).toContain('Settings')
  })

  it('shows notFound warning alert when fetchServiceDetail returns null', async () => {
    (fetchServiceDetail as any).mockResolvedValue(null)
    const router = makeRouter()
    await router.push('/services/missing?window=1h')
    await router.isReady()
    const w = mount(ServiceDetail, { global: { plugins: [router, makeI18n()] } })
    await flushPromises()
    expect(w.text()).toContain('Service not found')
  })

  it('shows error alert on fetch failure', async () => {
    (fetchServiceDetail as any).mockRejectedValue(new Error('boom-500'))
    const router = makeRouter()
    await router.push('/services/frontend?window=1h')
    await router.isReady()
    const w = mount(ServiceDetail, { global: { plugins: [router, makeI18n()] } })
    await flushPromises()
    expect(w.text()).toContain('boom-500')
  })
})
