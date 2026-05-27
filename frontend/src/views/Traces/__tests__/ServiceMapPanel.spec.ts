import { mount } from '@vue/test-utils'
import { describe, it, expect } from 'vitest'
import { createI18n } from 'vue-i18n'
import ServiceMapPanel from '../ServiceMapPanel.vue'

const i18n = createI18n({
  locale: 'en',
  legacy: false,
  messages: { en: { topology: { empty: 'No data' } } },
})

const mkSpan = (id: string, parent: string, service: string, status = 'Ok', dur = 1_000_000) => ({
  span_id: id,
  parent_span_id: parent,
  service,
  status,
  duration_ns: dur,
})

describe('ServiceMapPanel client-side derivation', () => {
  it('derives one node per unique service', () => {
    const w = mount(ServiceMapPanel, {
      global: { plugins: [i18n] },
      props: {
        spans: [
          mkSpan('s1', '', 'frontend'),
          mkSpan('s2', 's1', 'checkout'),
          mkSpan('s3', 's2', 'payment'),
          mkSpan('s4', 's2', 'checkout'),
        ],
      },
    })
    for (const svc of ['frontend', 'checkout', 'payment']) {
      expect(w.find(`[data-testid="graph-node-${svc}"]`).exists()).toBe(true)
    }
  })

  it('skips same-service parent-child as edge (no self-loops)', () => {
    const spans = [
      mkSpan('s1', '', 'checkout'),
      mkSpan('s2', 's1', 'checkout'),
      mkSpan('s3', 's2', 'payment'),
    ]
    const w = mount(ServiceMapPanel, { global: { plugins: [i18n] }, props: { spans } })
    // exactly 1 edge: checkout -> payment
    expect(w.findAll('line').length).toBe(1)
  })

  it('tolerates orphan parent_span_id (missing in span set)', () => {
    const spans = [
      mkSpan('s1', 'missing-parent', 'checkout'),
      mkSpan('s2', 's1', 'payment'),
    ]
    expect(() =>
      mount(ServiceMapPanel, { global: { plugins: [i18n] }, props: { spans } }),
    ).not.toThrow()
  })
})
