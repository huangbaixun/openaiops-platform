import { describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import { createI18n } from 'vue-i18n'
import { ref } from 'vue'
import LogsPanel from '../LogsPanel.vue'
import type { LogItem } from '../../api/logs'

const mockItems = ref<LogItem[]>([
  {
    ts: '2026-05-27T10:00:00Z',
    observed_ts: '',
    service: 'svc',
    severity_text: 'WARN',
    severity_number: 13,
    body: 'slow',
    trace_id: 'abc',
    span_id: 'sp1',
    trace_flags: 0,
    resource_attributes: {},
    attributes: {},
  },
])
const mockLoading = ref(false)
const mockedLoad = vi.fn(async () => undefined)

vi.mock('../../composables/useLogs', () => ({
  useLogsList: () => ({
    items: mockItems,
    hasMore: ref(false),
    loading: mockLoading,
    error: ref(null),
    load: mockedLoad,
  }),
}))

const i18n = createI18n({
  legacy: false,
  locale: 'en-US',
  messages: {
    'en-US': {
      logs: {
        tab: 'Logs',
        scopedToSpan: 'Showing span-scoped logs only',
        showAll: 'Show all logs for this trace',
        empty: 'No logs in the selected window',
      },
    },
  },
})

const router = createRouter({
  history: createMemoryHistory(),
  routes: [
    { path: '/traces/:id', component: { template: '<div/>' } },
    { path: '/', component: { template: '<div/>' } },
  ],
})

function makePlugins() {
  return [i18n, router, createPinia()]
}

describe('LogsPanel', () => {
  it('calls load with traceId + spanId and renders rows and scope chip', async () => {
    setActivePinia(createPinia())
    mockedLoad.mockClear()

    const w = mount(LogsPanel, {
      props: { traceId: 'abc', spanId: 'sp1' },
      global: { plugins: makePlugins() },
    })
    await flushPromises()

    expect(mockedLoad).toHaveBeenCalledWith(
      expect.objectContaining({ traceId: 'abc', spanId: 'sp1', limit: 200 }),
    )
    expect(w.find('[data-testid="logs-panel-scope"]').exists()).toBe(true)
    expect(w.find('[data-testid="logs-panel-list"]').exists()).toBe(true)
  })

  it('does not show scope chip when spanId is not provided', async () => {
    setActivePinia(createPinia())
    mockedLoad.mockClear()

    const w = mount(LogsPanel, {
      props: { traceId: 'abc' },
      global: { plugins: makePlugins() },
    })
    await flushPromises()

    expect(w.find('[data-testid="logs-panel-scope"]').exists()).toBe(false)
    expect(w.find('[data-testid="logs-panel-list"]').exists()).toBe(true)
  })

  it('calls load without spanId when spanId is null', async () => {
    setActivePinia(createPinia())
    mockedLoad.mockClear()

    mount(LogsPanel, {
      props: { traceId: 'abc', spanId: null },
      global: { plugins: makePlugins() },
    })
    await flushPromises()

    expect(mockedLoad).toHaveBeenCalledWith(
      expect.objectContaining({ traceId: 'abc', limit: 200 }),
    )
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const call = (mockedLoad.mock.calls as any[][])[0]?.[0] as Record<string, unknown>
    expect(call?.spanId).toBeUndefined()
  })

  it('emits clear-span when the clear button is clicked', async () => {
    setActivePinia(createPinia())
    mockedLoad.mockClear()

    const w = mount(LogsPanel, {
      props: { traceId: 'abc', spanId: 'sp1' },
      global: { plugins: makePlugins() },
    })
    await flushPromises()

    const scopeEl = w.find('[data-testid="logs-panel-scope"]')
    await scopeEl.find('button').trigger('click')
    expect(w.emitted('clear-span')).toBeTruthy()
  })
})
