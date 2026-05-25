import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import WaterfallChart from '../WaterfallChart.vue'
import type { SpanDetail } from '../../../api/traces'

function span(overrides: Partial<SpanDetail> = {}): SpanDetail {
  return {
    span_id: 'a',
    parent_span_id: '',
    service: 'fe',
    operation: 'GET /',
    ts: '2026-05-25T12:00:00.000Z',
    duration_ns: 1_000_000,
    status: 'Ok',
    span_kind: 'Server',
    resource_attributes: {},
    attributes: {},
    ...overrides,
  }
}

describe('WaterfallChart', () => {
  it('renders one rect per span', () => {
    const wrapper = mount(WaterfallChart, {
      props: {
        spans: [
          span({ span_id: 'a', service: 'fe' }),
          span({
            span_id: 'b',
            parent_span_id: 'a',
            service: 'be',
            ts: '2026-05-25T12:00:00.300Z',
            duration_ns: 400_000,
          }),
        ],
      },
    })
    expect(wrapper.findAll('[data-testid=waterfall-span]')).toHaveLength(2)
  })

  it('handles empty spans without throwing', () => {
    const wrapper = mount(WaterfallChart, { props: { spans: [] } })
    expect(wrapper.find('[data-testid=waterfall-svg]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid=waterfall-span]')).toHaveLength(0)
  })

  it('children render deeper (higher y) than parents', () => {
    const wrapper = mount(WaterfallChart, {
      props: {
        spans: [
          span({ span_id: 'root', parent_span_id: '', service: 'fe' }),
          span({
            span_id: 'child',
            parent_span_id: 'root',
            service: 'be',
            ts: '2026-05-25T12:00:00.100Z',
          }),
        ],
      },
    })
    const rects = wrapper.findAll('rect[data-testid=waterfall-span]')
    const rootY = Number(rects[0].attributes('y'))
    const childY = Number(rects[1].attributes('y'))
    expect(childY).toBeGreaterThan(rootY)
  })
})
