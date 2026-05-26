import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { createRouter, createMemoryHistory } from 'vue-router'
import LogsView from '../LogsView.vue'

vi.mock('../../../api/logs', () => ({
  listLogs: vi.fn().mockResolvedValue({ items: [], has_more: false }),
}))

const i18nMessages = {
  'en-US': {
    logs: {
      title: 'Logs',
      empty: 'No logs',
      hasMore: 'More results',
      filter: {
        service: 'Service',
        severity: 'Severity',
        traceId: 'Trace ID',
        spanId: 'Span ID',
        body: 'Body contains…',
        apply: 'Apply',
      },
    },
  },
}

function makeI18n() {
  return createI18n({ legacy: false, locale: 'en-US', messages: i18nMessages })
}

function makeRouter(initialPath = '/logs') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/logs', component: { template: '<div/>' } },
      { path: '/traces/:traceId', component: { template: '<div/>' } },
    ],
  })
  router.push(initialPath)
  return router
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

describe('LogsView', () => {
  it('renders the page title', async () => {
    const router = makeRouter()
    await router.isReady()
    const wrapper = mount(LogsView, {
      global: { plugins: [makeI18n(), router] },
    })
    expect(wrapper.find('[data-testid="logs-page"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('Logs')
  })

  it('renders filter inputs', async () => {
    const router = makeRouter()
    await router.isReady()
    const wrapper = mount(LogsView, {
      global: { plugins: [makeI18n(), router] },
    })
    expect(wrapper.find('[data-testid="filter-service"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="filter-severity"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="filter-trace-id"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="filter-span-id"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="filter-body"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="filter-apply"]').exists()).toBe(true)
  })

  it('shows empty state when no items returned', async () => {
    const { listLogs } = await import('../../../api/logs')
    ;(listLogs as ReturnType<typeof vi.fn>).mockResolvedValue({ items: [], has_more: false })

    const router = makeRouter()
    await router.isReady()
    const wrapper = mount(LogsView, {
      global: { plugins: [makeI18n(), router] },
    })
    // Wait for onMounted → load to settle
    await new Promise((r) => setTimeout(r, 50))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="logs-empty"]').exists()).toBe(true)
  })

  it('calls listLogs on mount', async () => {
    const { listLogs } = await import('../../../api/logs')
    const router = makeRouter()
    await router.isReady()
    mount(LogsView, {
      global: { plugins: [makeI18n(), router] },
    })
    await new Promise((r) => setTimeout(r, 50))
    expect(listLogs).toHaveBeenCalledTimes(1)
  })

  it('calls listLogs with service filter when Apply clicked', async () => {
    const { listLogs } = await import('../../../api/logs')
    ;(listLogs as ReturnType<typeof vi.fn>).mockResolvedValue({ items: [], has_more: false })

    const router = makeRouter()
    await router.isReady()
    const wrapper = mount(LogsView, {
      global: { plugins: [makeI18n(), router] },
    })
    // Set service filter value
    await wrapper.get('[data-testid="filter-service"]').find('input').setValue('myservice')
    vi.clearAllMocks()
    await wrapper.get('[data-testid="filter-apply"]').trigger('click')
    await new Promise((r) => setTimeout(r, 50))

    expect(listLogs).toHaveBeenCalledWith(
      expect.objectContaining({ service: ['myservice'] }),
    )
  })

  it('renders log rows when items returned', async () => {
    const { listLogs } = await import('../../../api/logs')
    ;(listLogs as ReturnType<typeof vi.fn>).mockResolvedValue({
      items: [
        {
          ts: '2026-05-26T10:00:00.000Z',
          observed_ts: '2026-05-26T10:00:00.001Z',
          service: 'backend',
          severity_text: 'ERROR',
          severity_number: 17,
          body: 'something failed',
          trace_id: 'trace001',
          span_id: 'span001',
          trace_flags: 1,
          resource_attributes: {},
          attributes: {},
        },
      ],
      has_more: false,
    })

    const router = makeRouter()
    await router.isReady()
    const wrapper = mount(LogsView, {
      global: { plugins: [makeI18n(), router] },
    })
    await new Promise((r) => setTimeout(r, 50))
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-testid="logs-list"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="log-row-trace001"]').exists()).toBe(true)
  })

  it('shows has-more text when has_more is true', async () => {
    const { listLogs } = await import('../../../api/logs')
    ;(listLogs as ReturnType<typeof vi.fn>).mockResolvedValue({
      items: [
        {
          ts: '2026-05-26T10:00:00.000Z',
          observed_ts: '2026-05-26T10:00:00.001Z',
          service: 'svc',
          severity_text: 'INFO',
          severity_number: 9,
          body: 'msg',
          trace_id: 'tid',
          span_id: '',
          trace_flags: 0,
          resource_attributes: {},
          attributes: {},
        },
      ],
      has_more: true,
    })

    const router = makeRouter()
    await router.isReady()
    const wrapper = mount(LogsView, {
      global: { plugins: [makeI18n(), router] },
    })
    await new Promise((r) => setTimeout(r, 50))
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('More results')
  })

  it('populates filter from URL query params', async () => {
    const router = makeRouter('/logs?service=mysvc&trace_id=abc')
    await router.isReady()
    const wrapper = mount(LogsView, {
      global: { plugins: [makeI18n(), router] },
    })
    // The filter-service input should be pre-filled
    expect((wrapper.get('[data-testid="filter-service"]').find('input').element as HTMLInputElement).value).toBe('mysvc')
    expect((wrapper.get('[data-testid="filter-trace-id"]').find('input').element as HTMLInputElement).value).toBe('abc')
  })
})
