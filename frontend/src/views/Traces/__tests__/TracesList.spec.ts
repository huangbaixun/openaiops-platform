import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { createRouter, createMemoryHistory } from 'vue-router'
import TracesList from '../TracesList.vue'

vi.mock('../../../api/traces', () => ({
  listTraces: vi.fn().mockResolvedValue({ items: [], has_more: false }),
}))

beforeEach(() => {
  setActivePinia(createPinia())
})

describe('TracesList', () => {
  it('passes filter params to listTraces on Apply', async () => {
    const { listTraces } = await import('../../../api/traces')
    const i18n = createI18n({
      legacy: false,
      locale: 'en-US',
      messages: {
        'en-US': {
          traces: {
            pageTitle: 'Traces',
            filterService: 'Service',
            filterOperation: 'Operation',
            filterMinDuration: 'Min duration (ms)',
            filterApply: 'Apply',
            colTraceId: 'Trace ID',
            colService: 'Service',
            colOperation: 'Operation',
            colStart: 'Start',
            colDuration: 'Duration',
            colSpanCount: 'Spans',
            empty: '-',
            hasMore: '-',
          },
        },
      },
    })
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/', component: { template: '<div/>' } }],
    })
    const wrapper = mount(TracesList, { global: { plugins: [i18n, router] } })

    // NaiveUI NInput wraps the actual <input>; drill in to set value.
    await wrapper.get('[data-testid=filter-service]').find('input').setValue('frontend')
    await wrapper.get('[data-testid=filter-apply]').trigger('click')

    expect(listTraces).toHaveBeenLastCalledWith(
      expect.objectContaining({
        service: 'frontend',
        limit: 100,
        sort: 'ts',
        order: 'desc',
      }),
    )
  })
})
