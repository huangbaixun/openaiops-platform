<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NEmpty } from 'naive-ui'
import { useForceSimulation } from './useForceSimulation'
import type { GraphNode, GraphEdge } from './types'
import type { Annotation } from '../../api/annotations'

const props = withDefaults(defineProps<{
  nodes: GraphNode[]
  edges: GraphEdge[]
  width?: number
  height?: number
  annByService?: Record<string, Annotation[]>
}>(), { width: 720, height: 480, annByService: () => ({}) })

function annCount(svc: string): number {
  return props.annByService[svc]?.length ?? 0
}

const emit = defineEmits<{ (e: 'node-click', n: GraphNode): void }>()

const { t } = useI18n()

const nodesRef = computed(() => props.nodes)
const edgesRef = computed(() => props.edges)
const { positions } = useForceSimulation(nodesRef, edgesRef, { width: props.width, height: props.height })

function pos(svc: string) { return positions.value[svc] ?? { x: 0, y: 0 } }
function radius(n: GraphNode): number {
  return Math.max(12, Math.min(40, Math.sqrt(n.calls || 1) * 2))
}
function strokeFor(e: GraphEdge): number {
  return Math.max(1, Math.min(6, Math.log((e.calls || 1) + 1)))
}
function nodeFill(n: GraphNode): string {
  const total = n.calls || 0
  const errs = n.errors || 0
  if (!total) return '#9ca3af'
  const rate = errs / total
  if (rate >= 0.05) return '#ef4444'
  if (rate >= 0.01) return '#f59e0b'
  return '#10b981'
}
function nodeStroke(n: GraphNode): string {
  return n.kind === 'external' ? '#6b7280' : '#1e3a8a'
}
function nodeDash(n: GraphNode): string {
  return n.kind === 'external' ? '4 2' : '0'
}
function edgeStroke(e: GraphEdge): string {
  const rate = e.calls > 0 ? e.errors / e.calls : 0
  if (rate >= 0.05) return '#ef4444'
  if (rate >= 0.01) return '#f59e0b'
  return '#6b7280'
}
</script>

<template>
  <div class="service-graph">
    <NEmpty v-if="!nodes.length" :description="t('topology.empty')" data-testid="graph-empty" />
    <svg v-else :viewBox="`0 0 ${width} ${height}`" :width="width" :height="height" data-testid="service-graph">
      <g class="edges">
        <line v-for="e in edges"
              :key="`${e.caller}-${e.callee}-${e.callee_kind}`"
              :x1="pos(e.caller).x" :y1="pos(e.caller).y"
              :x2="pos(e.callee).x" :y2="pos(e.callee).y"
              :stroke="edgeStroke(e)" :stroke-width="strokeFor(e)" stroke-opacity="0.6" />
      </g>
      <g class="nodes">
        <g v-for="n in nodes" :key="n.service"
           :data-testid="`graph-node-${n.service}`"
           @click="emit('node-click', n)" style="cursor: pointer;">
          <circle :cx="pos(n.service).x" :cy="pos(n.service).y"
                  :r="radius(n)" :fill="nodeFill(n)"
                  :stroke="nodeStroke(n)" :stroke-dasharray="nodeDash(n)" stroke-width="2" />
          <circle v-if="annCount(n.service) > 0"
                  :data-testid="`graph-node-ann-${n.service}`"
                  :cx="pos(n.service).x + radius(n) - 2"
                  :cy="pos(n.service).y - radius(n) + 2"
                  r="4" fill="#f0a020" stroke="#fff" stroke-width="1">
            <title>{{ annCount(n.service) }} AI annotation(s)</title>
          </circle>
          <text :x="pos(n.service).x"
                :y="pos(n.service).y + radius(n) + 14"
                text-anchor="middle" font-size="12" fill="#111827">{{ n.service }}</text>
        </g>
      </g>
    </svg>
  </div>
</template>

<style scoped>
.service-graph { width: 100%; height: 100%; }
</style>
