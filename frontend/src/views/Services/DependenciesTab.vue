<script setup lang="ts">
import { computed } from 'vue'
import type { HTMLAttributes } from 'vue'
import { NDataTable } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import type { ServiceDetail, Dependency } from '../../api/services'
import ServiceGraph from '../../components/ServiceGraph/ServiceGraph.vue'
import type { GraphNode, GraphEdge } from '../../components/ServiceGraph/types'

const props = defineProps<{ detail: ServiceDetail }>()
const { t } = useI18n()

const cols = [
  { title: 'Peer', key: 'peer' },
  { title: 'Kind', key: 'peer_kind' },
  { title: 'Calls', key: 'calls' },
  { title: 'Errors', key: 'errors' },
  { title: 'p95 (ms)', key: 'p95_ms', render: (r: Dependency) => r.p95_ms.toFixed(1) },
]

const graph = computed(() => {
  const nodes: GraphNode[] = [{ service: props.detail.service, kind: 'service', calls: (props.detail.stats.inbound.calls || 0) + (props.detail.stats.outbound.calls || 0) }]
  const edges: GraphEdge[] = []
  const seen = new Set([props.detail.service])
  for (const d of props.detail.dependencies.inbound) {
    if (!seen.has(d.peer)) { nodes.push({ service: d.peer, kind: d.peer_kind, calls: d.calls }); seen.add(d.peer) }
    edges.push({ caller: d.peer, callee: props.detail.service, callee_kind: 'service', calls: d.calls, errors: d.errors, p95_ms: d.p95_ms })
  }
  for (const d of props.detail.dependencies.outbound) {
    if (!seen.has(d.peer)) { nodes.push({ service: d.peer, kind: d.peer_kind, calls: d.calls }); seen.add(d.peer) }
    edges.push({ caller: props.detail.service, callee: d.peer, callee_kind: d.peer_kind, calls: d.calls, errors: d.errors, p95_ms: d.p95_ms })
  }
  return { nodes, edges }
})

function rowProps(r: Dependency): HTMLAttributes {
  // NaiveUI's CreateRowProps types as HTMLAttributes; data-* is valid HTML
  // but absent from Vue's typed HTMLAttributes — cast through unknown.
  return { 'data-testid': `dep-row-${r.peer}` } as unknown as HTMLAttributes
}
</script>
<template>
  <div class="deps">
    <ServiceGraph :nodes="graph.nodes" :edges="graph.edges" :width="600" :height="320" />
    <h3>{{ t('services.depInbound') }}</h3>
    <NDataTable :columns="cols" :data="detail.dependencies.inbound" :row-key="(r: Dependency) => `in-${r.peer}`" :row-props="rowProps" />
    <h3>{{ t('services.depOutbound') }}</h3>
    <NDataTable :columns="cols" :data="detail.dependencies.outbound" :row-key="(r: Dependency) => `out-${r.peer}`" :row-props="rowProps" />
  </div>
</template>
<style scoped>
.deps { padding: 16px; } h3 { margin-top: 16px; }
</style>
