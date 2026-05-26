import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import LogRow from '../LogRow.vue'
import type { LogItem } from '../../api/logs'

function makeLog(overrides: Partial<LogItem> = {}): LogItem {
  return {
    ts: '2026-05-26T10:00:00.000Z',
    observed_ts: '2026-05-26T10:00:00.001Z',
    service: 'frontend',
    severity_text: 'INFO',
    severity_number: 9,
    body: 'hello world',
    trace_id: 'abc1234567890123',
    span_id: 'def4567890123456',
    trace_flags: 1,
    resource_attributes: {},
    attributes: {},
    ...overrides,
  }
}

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div/>' } }],
  })
}

describe('LogRow', () => {
  it('renders service and severity', () => {
    const wrapper = mount(LogRow, {
      props: { log: makeLog() },
      global: { plugins: [makeRouter()] },
    })
    expect(wrapper.text()).toContain('frontend')
    expect(wrapper.text()).toContain('INFO')
  })

  it('renders truncated body (≤200 chars)', () => {
    const longBody = 'x'.repeat(250)
    const wrapper = mount(LogRow, {
      props: { log: makeLog({ body: longBody }) },
      global: { plugins: [makeRouter()] },
    })
    expect(wrapper.text()).toContain('x'.repeat(200) + '…')
    expect(wrapper.text()).not.toContain('x'.repeat(201) + '…')
  })

  it('does not truncate body shorter than 200 chars', () => {
    const body = 'short message'
    const wrapper = mount(LogRow, {
      props: { log: makeLog({ body }) },
      global: { plugins: [makeRouter()] },
    })
    expect(wrapper.text()).toContain(body)
    expect(wrapper.text()).not.toContain('…')
  })

  it('shows trace_id chip when trace_id present', () => {
    const wrapper = mount(LogRow, {
      props: { log: makeLog({ trace_id: 'abc1234567890123' }) },
      global: { plugins: [makeRouter()] },
    })
    const chip = wrapper.find('[data-testid="trace-link-abc1234567890123"]')
    expect(chip.exists()).toBe(true)
  })

  it('trace link points to /traces/:trace_id?focus_span=... when span_id present', () => {
    const wrapper = mount(LogRow, {
      props: {
        log: makeLog({ trace_id: 'abc1234567890123', span_id: 'def4567890123456' }),
      },
      global: { plugins: [makeRouter()] },
    })
    const chip = wrapper.find('[data-testid="trace-link-abc1234567890123"]')
    expect(chip.attributes('href')).toBe('/traces/abc1234567890123?focus_span=def4567890123456')
  })

  it('trace link omits focus_span when span_id is empty', () => {
    const wrapper = mount(LogRow, {
      props: { log: makeLog({ trace_id: 'abc1234567890123', span_id: '' }) },
      global: { plugins: [makeRouter()] },
    })
    const chip = wrapper.find('[data-testid="trace-link-abc1234567890123"]')
    expect(chip.attributes('href')).toBe('/traces/abc1234567890123')
  })

  it('does not render trace chip when trace_id is empty', () => {
    const wrapper = mount(LogRow, {
      props: { log: makeLog({ trace_id: '' }) },
      global: { plugins: [makeRouter()] },
    })
    expect(wrapper.find('[data-testid^="trace-link-"]').exists()).toBe(false)
  })

  it('has data-testid="log-row-{trace_id}" when trace_id present', () => {
    const wrapper = mount(LogRow, {
      props: { log: makeLog({ trace_id: 'abc1234567890123' }) },
      global: { plugins: [makeRouter()] },
    })
    expect(wrapper.find('[data-testid="log-row-abc1234567890123"]').exists()).toBe(true)
  })

  it('has data-testid="log-row-{ts}" when trace_id is empty', () => {
    const wrapper = mount(LogRow, {
      props: { log: makeLog({ trace_id: '', ts: '2026-05-26T10:00:00.000Z' }) },
      global: { plugins: [makeRouter()] },
    })
    expect(wrapper.find('[data-testid="log-row-2026-05-26T10:00:00.000Z"]').exists()).toBe(true)
  })

  it('toggles expanded view on click', async () => {
    const wrapper = mount(LogRow, {
      props: { log: makeLog({ body: 'click me' }) },
      global: { plugins: [makeRouter()] },
    })
    expect(wrapper.find('.log-row-detail').exists()).toBe(false)
    await wrapper.find('.log-row').trigger('click')
    expect(wrapper.find('.log-row-detail').exists()).toBe(true)
    await wrapper.find('.log-row').trigger('click')
    expect(wrapper.find('.log-row-detail').exists()).toBe(false)
  })

  it('pretty-prints valid JSON body when expanded', async () => {
    const jsonBody = '{"key":"value","num":42}'
    const wrapper = mount(LogRow, {
      props: { log: makeLog({ body: jsonBody }) },
      global: { plugins: [makeRouter()] },
    })
    await wrapper.find('.log-row').trigger('click')
    const pre = wrapper.find('.log-body-full')
    expect(pre.text()).toContain('"key"')
    expect(pre.text()).toContain('"value"')
  })

  it('shows raw body when not valid JSON', async () => {
    const rawBody = 'plain text log message'
    const wrapper = mount(LogRow, {
      props: { log: makeLog({ body: rawBody }) },
      global: { plugins: [makeRouter()] },
    })
    await wrapper.find('.log-row').trigger('click')
    const pre = wrapper.find('.log-body-full')
    expect(pre.text()).toBe(rawBody)
  })

  it('shows attributes section when non-empty', async () => {
    const wrapper = mount(LogRow, {
      props: {
        log: makeLog({ attributes: { 'http.method': 'GET', 'http.status_code': '200' } }),
      },
      global: { plugins: [makeRouter()] },
    })
    await wrapper.find('.log-row').trigger('click')
    expect(wrapper.text()).toContain('Attributes')
    expect(wrapper.text()).toContain('http.method')
  })

  it('hides attributes section when empty', async () => {
    const wrapper = mount(LogRow, {
      props: { log: makeLog({ attributes: {} }) },
      global: { plugins: [makeRouter()] },
    })
    await wrapper.find('.log-row').trigger('click')
    const sections = wrapper.findAll('.log-detail-section')
    const attrSection = sections.find((s) => s.text().includes('Attributes') && s.text().includes('http'))
    expect(attrSection).toBeUndefined()
  })
})
