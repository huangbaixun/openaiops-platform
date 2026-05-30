import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { createRouter, createWebHistory } from 'vue-router'
import SideBar from '../SideBar.vue'

const i18n = createI18n({ legacy: false, locale: 'en-US', messages: { 'en-US': {} } })
const router = createRouter({ history: createWebHistory(), routes: [
  { path: '/overview', name: 'overview', component: { template: '<div/>' } },
  { path: '/traces', name: 'traces', component: { template: '<div/>' } },
  { path: '/logs', name: 'logs', component: { template: '<div/>' } },
  { path: '/topology', name: 'topology', component: { template: '<div/>' } },
] })

function mountSidebar() {
  return mount(SideBar, { global: { plugins: [i18n, router] } })
}

describe('SideBar', () => {
  it('renders the three OpenAPM nav groups', () => {
    const w = mountSidebar()
    const labels = w.findAll('.nav-group .label').map(n => n.text())
    expect(labels).toEqual(['OBSERVE', 'ANALYZE', 'PLATFORM'])
  })

  it('real pages are RouterLinks; backend-less items are disabled placeholders', () => {
    const w = mountSidebar()
    expect(w.find('[data-testid="nav-overview"]').exists()).toBe(true)
    expect(w.find('[data-testid="nav-traces"]').exists()).toBe(true)
    expect(w.find('[data-testid="nav-logs"]').exists()).toBe(true)
    expect(w.find('[data-testid="nav-topology"]').exists()).toBe(true)
    const ex = w.find('[data-testid="nav-exceptions"]')
    expect(ex.exists()).toBe(true)
    expect(ex.classes()).toContain('disabled')
    expect(ex.element.tagName).not.toBe('A')
  })

  it('toggle button is present and clickable', async () => {
    const w = mountSidebar()
    await w.find('.sidebar-toggle').trigger('click')
    expect(w.find('.sidebar-toggle').exists()).toBe(true)
  })
})
