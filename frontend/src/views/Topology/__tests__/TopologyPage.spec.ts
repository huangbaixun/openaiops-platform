import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createI18n } from 'vue-i18n'
import TopologyPage from '../TopologyPage.vue'

vi.mock('../../../api/topology', () => ({
  fetchTopology: vi.fn(),
}))
import { fetchTopology } from '../../../api/topology'

function makeI18n() {
  return createI18n({
    locale: 'en', legacy: false,
    messages: {
      en: {
        topology: { title: 'Topology', empty: 'No data' },
        timeWindow: { '15m': '15m', '1h': '1h', '6h': '6h', '24h': '24h' },
      },
    },
  })
}

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/topology', component: TopologyPage },
      { path: '/services/:name', component: { template: '<div/>' } },
    ],
  })
}

describe('TopologyPage', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('renders graph nodes after fetch', async () => {
    (fetchTopology as any).mockResolvedValue({
      window: '1h',
      nodes: [
        { service: 'frontend', kind: 'service', calls: 100, errors: 1, p95_ms: 12 },
        { service: 'checkout', kind: 'service', calls: 50, errors: 0, p95_ms: 30 },
      ],
      edges: [
        { caller: 'frontend', callee: 'checkout', callee_kind: 'service', calls: 50, errors: 0, p95_ms: 30 },
      ],
    })
    const router = makeRouter()
    await router.push('/topology?window=1h')
    await router.isReady()
    const w = mount(TopologyPage, { global: { plugins: [router, makeI18n()] } })
    await flushPromises()
    expect(w.find('[data-testid="graph-node-frontend"]').exists()).toBe(true)
    expect(w.find('[data-testid="graph-node-checkout"]').exists()).toBe(true)
  })

  it('navigates to /services/:name when a service node is clicked', async () => {
    (fetchTopology as any).mockResolvedValue({
      window: '1h',
      nodes: [{ service: 'frontend', kind: 'service', calls: 100, errors: 0, p95_ms: 10 }],
      edges: [],
    })
    const router = makeRouter()
    await router.push('/topology?window=1h')
    await router.isReady()
    const w = mount(TopologyPage, { global: { plugins: [router, makeI18n()] } })
    await flushPromises()
    const node = w.find('[data-testid="graph-node-frontend"]')
    expect(node.exists()).toBe(true)
    await node.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/services/frontend')
  })
})
