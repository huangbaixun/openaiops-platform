import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import DependenciesTab from '../DependenciesTab.vue'
import type { ServiceDetail } from '../../../api/services'

function makeI18n() {
  return createI18n({
    locale: 'en', legacy: false,
    messages: {
      en: {
        services: { depInbound: 'Inbound', depOutbound: 'Outbound' },
        topology: { empty: 'No data' },
      },
    },
  })
}

describe('DependenciesTab', () => {
  it('renders inbound and outbound dependency rows', () => {
    const detail: ServiceDetail = {
      service: 'frontend',
      window: '1h',
      stats: {
        inbound: { calls: 100, errors: 1, error_rate: 0.01, p95_ms: 12.3 },
        outbound: { calls: 50, errors: 0, error_rate: 0, p95_ms: 8.0 },
      },
      dependencies: {
        inbound: [
          { peer: 'gateway', peer_kind: 'service', calls: 80, errors: 0, p95_ms: 5.0 },
        ],
        outbound: [
          { peer: 'checkout', peer_kind: 'service', calls: 30, errors: 0, p95_ms: 9.0 },
          { peer: 'stripe-api', peer_kind: 'external', calls: 20, errors: 1, p95_ms: 50.0 },
        ],
      },
    }
    const w = mount(DependenciesTab, {
      props: { detail },
      global: { plugins: [makeI18n()] },
    })
    expect(w.find('[data-testid="dep-row-gateway"]').exists()).toBe(true)
    expect(w.find('[data-testid="dep-row-checkout"]').exists()).toBe(true)
    expect(w.find('[data-testid="dep-row-stripe-api"]').exists()).toBe(true)
  })
})
