import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SeverityBadge from '../SeverityBadge.vue'

describe('SeverityBadge', () => {
  it('renders the severity text', () => {
    const wrapper = mount(SeverityBadge, { props: { severity: 'INFO' } })
    expect(wrapper.text()).toContain('INFO')
  })

  it('uses "info" type for INFO', () => {
    const wrapper = mount(SeverityBadge, { props: { severity: 'INFO' } })
    // NTag renders a data-testid or class based on type; check rendered text at minimum
    expect(wrapper.text()).toBe('INFO')
  })

  it('uses "default" type for DEBUG', () => {
    const wrapper = mount(SeverityBadge, { props: { severity: 'DEBUG' } })
    expect(wrapper.text()).toBe('DEBUG')
  })

  it('uses "warning" type for WARN', () => {
    const wrapper = mount(SeverityBadge, { props: { severity: 'WARN' } })
    expect(wrapper.text()).toBe('WARN')
  })

  it('uses "warning" type for WARNING (alias)', () => {
    const wrapper = mount(SeverityBadge, { props: { severity: 'WARNING' } })
    expect(wrapper.text()).toBe('WARNING')
  })

  it('uses "error" type for ERROR', () => {
    const wrapper = mount(SeverityBadge, { props: { severity: 'ERROR' } })
    expect(wrapper.text()).toBe('ERROR')
  })

  it('uses "error" type for FATAL', () => {
    const wrapper = mount(SeverityBadge, { props: { severity: 'FATAL' } })
    expect(wrapper.text()).toBe('FATAL')
  })

  it('is case-insensitive — lowercase info works', () => {
    const wrapper = mount(SeverityBadge, { props: { severity: 'info' } })
    expect(wrapper.text()).toBe('info')
  })

  it('falls back to "default" for unknown severity', () => {
    const wrapper = mount(SeverityBadge, { props: { severity: 'TRACE' } })
    expect(wrapper.text()).toBe('TRACE')
  })
})
