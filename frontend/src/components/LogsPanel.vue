<script setup lang="ts">
import { watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NSpin, NEmpty, NButton, NAlert } from 'naive-ui'
import { useLogsList } from '../composables/useLogs'
import LogRow from './LogRow.vue'

const props = defineProps<{ traceId: string; spanId?: string | null }>()
const emit = defineEmits<{ (e: 'clear-span'): void }>()

const { t } = useI18n()
const { items, loading, error, load } = useLogsList()

function fetch() {
  load({ traceId: props.traceId, spanId: props.spanId || undefined, limit: 200 })
}

// Two scalar watches read clearer than a single getter returning [a,b] —
// Vue 3's array-getter compare works, but intent is explicit this way.
watch(() => props.traceId, fetch, { immediate: true })
watch(() => props.spanId, fetch)
</script>

<template>
  <div class="logs-panel">
    <div v-if="spanId" class="logs-panel__scope" data-testid="logs-panel-scope">
      <span>{{ t('logs.scopedToSpan') }}: {{ spanId }}</span>
      <NButton size="tiny" @click="emit('clear-span')">{{ t('logs.showAll') }}</NButton>
    </div>
    <NAlert v-if="error" type="error" :title="t('logs.errorTitle')" data-testid="logs-panel-error">
      {{ error }}
    </NAlert>
    <NSpin :show="loading">
      <NEmpty v-if="!items.length && !error" :description="t('logs.empty')" />
      <div v-else data-testid="logs-panel-list">
        <LogRow v-for="(log, i) in items" :key="`${log.ts}|${log.trace_id}|${log.span_id}|${i}`" :log="log" />
      </div>
    </NSpin>
  </div>
</template>

<style scoped>
.logs-panel { padding: 8px; }
.logs-panel__scope {
  padding: 6px 8px;
  background: var(--color-warning-bg, #fff3cd);
  display: flex;
  gap: 12px;
  align-items: center;
  margin-bottom: 8px;
}
</style>
