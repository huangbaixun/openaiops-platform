<script setup lang="ts">
import type { ServiceDetail } from '../../api/services'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
defineProps<{ detail: ServiceDetail }>()
const { t } = useI18n()
function ms(n?: number): string { return n != null ? n.toFixed(1) + 'ms' : '—' }
function pct(n?: number): string { return n != null ? (n * 100).toFixed(2) + '%' : '—' }
</script>
<template>
  <div class="signals">
    <div class="grid-3">
      <div class="card kpi" data-testid="signals-inbound-calls">
        <div class="name">{{ t('services.calls') }}</div>
        <div class="value">{{ detail.stats.inbound.calls }}</div>
      </div>
      <div class="card kpi" data-testid="signals-inbound-errors">
        <div class="name">{{ t('services.errors') }}</div>
        <div class="value">{{ pct(detail.stats.inbound.error_rate) }}</div>
      </div>
      <div class="card kpi" data-testid="signals-inbound-p95">
        <div class="name">p95</div>
        <div class="value">{{ ms(detail.stats.inbound.p95_ms) }}</div>
      </div>
    </div>
    <p class="quick">
      <RouterLink :to="`/traces?service=${detail.service}&window=${detail.window}`">{{ t('services.viewTraces') }}</RouterLink>
      ·
      <RouterLink :to="`/logs?service=${detail.service}&window=${detail.window}`">{{ t('services.viewLogs') }}</RouterLink>
    </p>
  </div>
</template>
<style scoped>
.signals { padding: 16px; }
.quick { margin-top: 16px; }
</style>
