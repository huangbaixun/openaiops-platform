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
    <div class="strip">
      <div data-testid="signals-inbound-calls"><span>{{ t('services.calls') }}</span><b>{{ detail.stats.inbound.calls }}</b></div>
      <div data-testid="signals-inbound-errors"><span>{{ t('services.errors') }}</span><b>{{ pct(detail.stats.inbound.error_rate) }}</b></div>
      <div data-testid="signals-inbound-p95"><span>p95</span><b>{{ ms(detail.stats.inbound.p95_ms) }}</b></div>
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
.strip { display: flex; gap: 32px; }
.strip > div { display: flex; flex-direction: column; }
.strip span { font-size: 12px; color: var(--text-3); }
.strip b { font-size: 24px; }
.quick { margin-top: 16px; }
</style>
