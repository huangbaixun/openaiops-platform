<script setup lang="ts">
import type { ServiceListItem } from '../../api/services'
defineProps<{ item: ServiceListItem }>()
const emit = defineEmits<{ (e: 'click', s: string): void }>()
function ring(rate: number): string {
  if (rate >= 0.05) return '#ef4444'
  if (rate >= 0.01) return '#f59e0b'
  return '#10b981'
}
function pct(n: number): string { return (n * 100).toFixed(2) + '%' }
function ms(n: number): string { return n.toFixed(1) + 'ms' }
</script>
<template>
  <div class="card" :data-testid="`service-card-${item.service}`" @click="emit('click', item.service)">
    <div class="header">
      <div class="name">{{ item.service }}</div>
      <div class="ring" :style="{ background: ring(item.inbound_error_rate) }" />
    </div>
    <div class="stats">
      <div><span>calls</span><b>{{ item.inbound_calls }}</b></div>
      <div data-testid="card-error-rate"><span>err</span><b>{{ pct(item.inbound_error_rate) }}</b></div>
      <div><span>p95</span><b>{{ ms(item.inbound_p95_ms) }}</b></div>
    </div>
  </div>
</template>
<style scoped>
.card { padding: 16px; border: 1px solid var(--border); border-radius: 8px; background: var(--bg-card); cursor: pointer; }
.card:hover { border-color: var(--primary); }
.header { display: flex; align-items: center; gap: 8px; }
.name { font-weight: 600; flex: 1; }
.ring { width: 12px; height: 12px; border-radius: 50%; }
.stats { display: flex; gap: 16px; margin-top: 8px; font-size: 12px; }
.stats span { color: var(--text-3); margin-right: 4px; }
</style>
