import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createI18n } from 'vue-i18n'
import SignalsTab from '../SignalsTab.vue'
import type { ServiceDetail } from '../../../api/services'

function makeI18n() {
  return createI18n({
    locale: 'en', legacy: false,
    messages: {
      en: {
        services: {
          calls: 'Calls',
          errors: 'Errors',
          viewTraces: 'View traces',
          viewLogs: 'View logs',
        },
      },
    },
  })
}

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div/>' } },
      { path: '/traces', component: { template: '<div/>' } },
      { path: '/logs', component: { template: '<div/>' } },
    ],
  })
}

describe('SignalsTab', () => {
  it('renders the three inbound metric strip cells', async () => {
    const detail: ServiceDetail = {
      service: 'frontend',
      window: '1h',
      stats: {
        inbound: { calls: 100, errors: 1, error_rate: 0.01, p95_ms: 12.3 },
        outbound: { calls: 50, errors: 0, error_rate: 0, p95_ms: 8.0 },
      },
      dependencies: { inbound: [], outbound: [] },
    }
    const router = makeRouter()
    await router.push('/')
    await router.isReady()
    const w = mount(SignalsTab, {
      props: { detail },
      global: { plugins: [router, makeI18n()] },
    })
    expect(w.find('[data-testid="signals-inbound-calls"]').exists()).toBe(true)
    expect(w.find('[data-testid="signals-inbound-errors"]').exists()).toBe(true)
    expect(w.find('[data-testid="signals-inbound-p95"]').exists()).toBe(true)
    expect(w.find('[data-testid="signals-inbound-p95"]').text()).toContain('12.3ms')
  })
})
