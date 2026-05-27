<script setup lang="ts">
import { computed } from 'vue'
import ServiceGraph from '../../components/ServiceGraph/ServiceGraph.vue'
import type { GraphNode, GraphEdge } from '../../components/ServiceGraph/types'

interface Span {
  span_id: string
  parent_span_id: string
  service: string
  status: string
  duration_ns: number
}

const props = defineProps<{ spans: Span[] }>()
const emit = defineEmits<{ (e: 'node-click', n: GraphNode): void }>()

const graph = computed(() => {
  const spanIndex = new Map(props.spans.map((s) => [s.span_id, s]))
  const nodeMap = new Map<string, GraphNode>()
  const edges: GraphEdge[] = []
  const seenEdge = new Set<string>()
  for (const s of props.spans) {
    if (!nodeMap.has(s.service)) {
      nodeMap.set(s.service, { service: s.service, kind: 'service', calls: 0, errors: 0 })
    }
    const n = nodeMap.get(s.service)!
    n.calls = (n.calls ?? 0) + 1
    if (s.status === 'Error') n.errors = (n.errors ?? 0) + 1
    if (!s.parent_span_id) continue
    const p = spanIndex.get(s.parent_span_id)
    if (!p) continue // orphan: don't synthesize edge
    if (p.service === s.service) continue // same-service parent: not an edge
    const key = `${p.service}|${s.service}`
    if (seenEdge.has(key)) {
      const e = edges.find((x) => x.caller === p.service && x.callee === s.service)!
      e.calls += 1
      if (s.status === 'Error') e.errors += 1
    } else {
      edges.push({
        caller: p.service,
        callee: s.service,
        callee_kind: 'service',
        calls: 1,
        errors: s.status === 'Error' ? 1 : 0,
        p95_ms: s.duration_ns / 1_000_000,
      })
      seenEdge.add(key)
    }
  }
  return { nodes: [...nodeMap.values()], edges }
})
</script>

<template>
  <ServiceGraph
    :nodes="graph.nodes"
    :edges="graph.edges"
    :width="720"
    :height="420"
    @node-click="emit('node-click', $event)"
  />
</template>
