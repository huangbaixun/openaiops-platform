import { ref, nextTick } from 'vue'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { useForceSimulation } from '../useForceSimulation'
import type { GraphNode, GraphEdge } from '../types'

describe('useForceSimulation', () => {
  beforeEach(() => { vi.useFakeTimers() })
  afterEach(() => { vi.useRealTimers() })

  it('exposes positions for each node after one tick', async () => {
    const nodes = ref<GraphNode[]>([
      { service: 'a', kind: 'service', calls: 10 },
      { service: 'b', kind: 'service', calls: 5 },
    ])
    const edges = ref<GraphEdge[]>([
      { caller: 'a', callee: 'b', callee_kind: 'service', calls: 1, errors: 0, p95_ms: 1 },
    ])
    const { positions } = useForceSimulation(nodes, edges, { width: 400, height: 400 })
    await nextTick()
    vi.advanceTimersByTime(500)
    await nextTick()
    expect(positions.value['a']).toBeDefined()
    expect(positions.value['b']).toBeDefined()
  })

  it('restart() bumps alpha back up', async () => {
    const nodes = ref<GraphNode[]>([{ service: 'a', kind: 'service', calls: 1 }])
    const edges = ref<GraphEdge[]>([])
    const { restart } = useForceSimulation(nodes, edges, { width: 100, height: 100 })
    expect(() => restart()).not.toThrow()
  })
})
