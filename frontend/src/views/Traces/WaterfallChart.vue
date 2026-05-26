<script setup lang="ts">
import { computed } from 'vue'
import type { SpanDetail } from '../../api/traces'

const props = defineProps<{ spans: SpanDetail[] }>()
const emit = defineEmits<{ (e: 'span-click', spanId: string): void }>()

interface Laid {
  span: SpanDetail
  depth: number
  x: number
  width: number
  y: number
  fill: string
}

const ROW_H = 22
const PAD_TOP = 8
const WIDTH = 900

function colorFor(service: string): string {
  let h = 0
  for (const c of service) {
    h = (h * 31 + c.charCodeAt(0)) % 360
  }
  return `hsl(${h}, 55%, 62%)`
}

const layout = computed<{ rows: Laid[]; total: number; startNs: number; height: number }>(() => {
  if (props.spans.length === 0) {
    return { rows: [], total: 0, startNs: 0, height: ROW_H + PAD_TOP }
  }
  // Resolve start in ns by parsing the RFC3339 ts and combining with sub-ms precision.
  // JS Date can lose sub-ms, but for layout this is good enough at the millisecond level.
  const tsNs = props.spans.map((s) => Date.parse(s.ts) * 1_000_000)
  const startNs = Math.min(...tsNs)
  const endNs = Math.max(...props.spans.map((s, i) => tsNs[i] + s.duration_ns))
  const total = endNs - startNs || 1
  const byId = new Map(props.spans.map((s) => [s.span_id, s]))
  const depthCache = new Map<string, number>()
  function depthOf(s: SpanDetail): number {
    const cached = depthCache.get(s.span_id)
    if (cached !== undefined) return cached
    const parent = s.parent_span_id ? byId.get(s.parent_span_id) : undefined
    const v = parent ? depthOf(parent) + 1 : 0
    depthCache.set(s.span_id, v)
    return v
  }
  const rows: Laid[] = props.spans.map((s, i) => {
    const offsetNs = tsNs[i] - startNs
    const d = depthOf(s)
    return {
      span: s,
      depth: d,
      x: (offsetNs / total) * WIDTH,
      width: Math.max(2, (s.duration_ns / total) * WIDTH),
      y: PAD_TOP + d * ROW_H,
      fill: colorFor(s.service),
    }
  })
  const maxDepth = rows.reduce((m, r) => Math.max(m, r.depth), 0)
  const height = PAD_TOP + (maxDepth + 1) * ROW_H + PAD_TOP
  return { rows, total, startNs, height }
})
</script>

<template>
  <svg
    :width="WIDTH"
    :height="layout.height"
    data-testid="waterfall-svg"
    role="img"
  >
    <g v-for="row in layout.rows" :key="row.span.span_id">
      <rect
        :x="row.x"
        :y="row.y"
        :width="row.width"
        :height="ROW_H - 4"
        :fill="row.fill"
        data-testid="waterfall-span"
        style="cursor: pointer"
        @click="emit('span-click', row.span.span_id)"
      >
        <title>
          {{ row.span.service }} · {{ row.span.operation }} ·
          {{ (row.span.duration_ns / 1_000_000).toFixed(2) }} ms
        </title>
      </rect>
      <text
        :x="row.x + row.width + 4"
        :y="row.y + ROW_H - 8"
        font-size="11"
        fill="#cbd5e1"
      >
        {{ row.span.operation }}
      </text>
    </g>
  </svg>
</template>
