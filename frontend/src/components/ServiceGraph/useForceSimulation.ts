import { type Ref, watchEffect, onScopeDispose, ref } from 'vue'
import {
  forceSimulation,
  forceManyBody,
  forceLink,
  forceCenter,
  forceCollide,
  type Simulation,
} from 'd3-force'
import type { GraphNode, GraphEdge, SimEdge } from './types'

export interface SimOpts { width: number; height: number }

export function useForceSimulation(
  nodes: Ref<GraphNode[]>,
  edges: Ref<GraphEdge[]>,
  opts: SimOpts,
) {
  const positions = ref<Record<string, { x: number; y: number }>>({})
  let sim: Simulation<GraphNode, SimEdge> | null = null

  function radiusFor(n: GraphNode): number {
    return Math.max(12, Math.min(40, Math.sqrt(n.calls || 1) * 2))
  }

  watchEffect(() => {
    sim?.stop()
    // Clone to avoid disturbing Vue reactivity — d3-force mutates input.
    // Map caller/callee -> source/target so forceLink can resolve via id().
    const nodeData = nodes.value.map(n => ({ ...n }))
    const edgeData: SimEdge[] = edges.value.map(e => ({
      ...e,
      source: e.caller,
      target: e.callee,
    }))
    sim = forceSimulation<GraphNode>(nodeData)
      .force('charge', forceManyBody().strength(-300))
      .force('link', forceLink<GraphNode, SimEdge>(edgeData)
        .id((d: GraphNode) => d.service)
        .distance(80))
      .force('center', forceCenter(opts.width / 2, opts.height / 2))
      .force('collide', forceCollide<GraphNode>(d => radiusFor(d) + 4))
      .on('tick', () => {
        const next: Record<string, { x: number; y: number }> = {}
        for (const n of nodeData) {
          if (n.x != null && n.y != null) next[n.service] = { x: n.x, y: n.y }
        }
        positions.value = next
      })
  })

  onScopeDispose(() => { sim?.stop() })

  return {
    positions,
    restart: () => sim?.alpha(1).restart(),
  }
}
