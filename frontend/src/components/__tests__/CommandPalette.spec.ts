import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import CommandPalette from '../CommandPalette.vue'

const push = vi.fn()
vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))
vi.mock('../../api/services', () => ({
  fetchServices: vi.fn().mockResolvedValue({ items: [{ service: 'checkout' }, { service: 'payment' }] }),
}))
vi.mock('../../composables/useTimeWindow', () => ({ useTimeWindow: () => ({ windowVal: { value: '1h' } }) }))

describe('CommandPalette', () => {
  beforeEach(() => { setActivePinia(createPinia()); push.mockClear() })

  it('opens, fuzzy-filters services, and navigates on Enter', async () => {
    const { useCommandPalette } = await import('../../composables/useCommandPalette')
    const w = mount(CommandPalette, { global: { plugins: [] } })
    useCommandPalette().openPalette()
    await flushPromises()
    const input = w.get('input')
    await input.setValue('check')
    await input.trigger('keydown', { key: 'Enter' })
    expect(push).toHaveBeenCalledWith('/services/checkout')
  })
})
