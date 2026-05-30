<script setup lang="ts">
import { computed } from 'vue'
import type { ServiceListItem } from '../../api/services'
const props = defineProps<{ item: ServiceListItem }>()
const emit = defineEmits<{ (e: 'click', s: string): void }>()
const ringColor = computed(() => {
  if (props.item.inbound_error_rate > 0.05) return 'var(--danger)'
  if (props.item.inbound_error_rate > 0.01) return 'var(--warning)'
  return 'var(--success)'
})
function pct(n: number): string { return (n * 100).toFixed(2) + '%' }
function ms(n: number): string { return n.toFixed(1) }
</script>
<template>
  <div class="card svc-card" :data-testid="`service-card-${item.service}`" @click="emit('click', item.service)">
    <div class="svc-head">
      <span class="svc-name">{{ item.service }}</span>
      <span class="health-ring" :style="{ '--c': ringColor }" />
    </div>
    <div class="svc-kpis">
      <div class="kpi"><div class="name">calls</div><div class="value">{{ item.inbound_calls }}</div></div>
      <div class="kpi" data-testid="card-error-rate"><div class="name">err</div><div class="value">{{ pct(item.inbound_error_rate) }}</div></div>
      <div class="kpi"><div class="name">p95</div><div class="value">{{ ms(item.inbound_p95_ms) }}<span class="unit">ms</span></div></div>
    </div>
  </div>
</template>
<style scoped>
.svc-card { cursor: pointer; transition: box-shadow .15s, transform .15s; }
.svc-card:hover { box-shadow: var(--shadow-md); transform: translateY(-1px); }
.svc-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; }
.svc-name { font-weight: 600; font-size: 14px; }
.health-ring { width: 12px; height: 12px; border-radius: 50%; background: var(--c); box-shadow: 0 0 0 4px color-mix(in srgb, var(--c) 22%, transparent); }
.svc-kpis { display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; }
.svc-kpis .kpi .value { font-size: 18px; }
</style>
