import { mount } from '@vue/test-utils'
import { describe, it, expect } from 'vitest'
import { createI18n } from 'vue-i18n'
import ServiceGraph from '../ServiceGraph.vue'

const i18n = createI18n({
  locale: 'en', legacy: false,
  messages: { en: { topology: { empty: 'No data' } } },
})

describe('ServiceGraph', () => {
  it('renders one node per service in props', () => {
    const w = mount(ServiceGraph, {
      props: {
        nodes: [
          { service: 'frontend', kind: 'service', calls: 10 },
          { service: 'checkout', kind: 'service', calls: 5 },
        ],
        edges: [],
      },
      global: { plugins: [i18n] },
    })
    expect(w.find('[data-testid="graph-node-frontend"]').exists()).toBe(true)
    expect(w.find('[data-testid="graph-node-checkout"]').exists()).toBe(true)
  })

  it('shows NEmpty when nodes is empty', () => {
    const w = mount(ServiceGraph, {
      props: { nodes: [], edges: [] },
      global: { plugins: [i18n] },
    })
    expect(w.find('[data-testid="graph-empty"]').exists()).toBe(true)
  })

  it('emits node-click on node click', async () => {
    const w = mount(ServiceGraph, {
      props: {
        nodes: [{ service: 'svc', kind: 'service', calls: 1 }],
        edges: [],
      },
      global: { plugins: [i18n] },
    })
    await w.find('[data-testid="graph-node-svc"]').trigger('click')
    expect(w.emitted('node-click')).toBeTruthy()
  })
})
