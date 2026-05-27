import { describe, it, expect } from 'vitest'
import { defineComponent, h, nextTick, watch } from 'vue'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { useTimeWindow } from '../useTimeWindow'

async function makeHarness(initialPath = '/topology') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/topology', component: { template: '<div/>' } }],
  })
  await router.push(initialPath)
  await router.isReady()
  const seen: string[] = []
  const Test = defineComponent({
    setup() {
      const { windowVal, apply } = useTimeWindow()
      watch(windowVal, (v) => seen.push(v))
      return { apply, windowVal }
    },
    render() {
      return h('div')
    },
  })
  const w = mount(Test, { global: { plugins: [router] } })
  return { w, router, seen }
}

describe('useTimeWindow suppressNextWatch', () => {
  it('apply does NOT cause double-emit via router -> watch', async () => {
    const { w, seen } = await makeHarness('/topology?window=1h')
    expect((w.vm as unknown as { windowVal: string }).windowVal).toBe('1h')
    ;(w.vm as unknown as { apply: (v: string) => void }).apply('6h')
    await nextTick()
    await nextTick()
    // Watcher should observe exactly one '6h' transition.
    expect(seen.filter((v) => v === '6h')).toHaveLength(1)
  })

  it('navigating to ?window=24h externally updates windowVal', async () => {
    const { w, router } = await makeHarness('/topology?window=1h')
    await router.replace({ path: '/topology', query: { window: '24h' } })
    await nextTick()
    expect((w.vm as unknown as { windowVal: string }).windowVal).toBe('24h')
  })

  it('invalid query falls back to default', async () => {
    const { w } = await makeHarness('/topology?window=bogus')
    expect((w.vm as unknown as { windowVal: string }).windowVal).toBe('1h')
  })
})
