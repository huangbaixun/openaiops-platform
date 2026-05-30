<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { NCard, NInput, NSelect, NButton, NSpace, NText, NSpin, NDatePicker, NAlert } from 'naive-ui'
import { useLogsList } from '../../composables/useLogs'
import LogRow from '../../components/LogRow.vue'
import { useTimeWindow } from '../../composables/useTimeWindow'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const { items, hasMore, loading, error, load } = useLogsList()
const { refreshTick } = useTimeWindow()

function readQueryArray(q: unknown): string[] {
  if (q == null) return []
  // route.query value can be string | string[] | null | (string|null)[]
  const arr = Array.isArray(q) ? q : [q]
  return arr.filter((v): v is string => typeof v === 'string' && v.length > 0)
}

// Filter state (local refs, synced to URL on Apply)
const filterService = ref<string>((route.query['service'] as string) ?? '')
const filterSeverity = ref<string[]>(readQueryArray(route.query['severity']))
const filterTraceId = ref<string>((route.query['trace_id'] as string) ?? '')
const filterSpanId = ref<string>((route.query['span_id'] as string) ?? '')
const filterBodyContains = ref<string>((route.query['body_contains'] as string) ?? '')
const filterTsRange = ref<[number, number] | null>(null)
{
  const f = typeof route.query['ts_from'] === 'string' ? Date.parse(route.query['ts_from'] as string) : NaN
  const tt = typeof route.query['ts_to'] === 'string' ? Date.parse(route.query['ts_to'] as string) : NaN
  if (Number.isFinite(f) && Number.isFinite(tt)) filterTsRange.value = [f, tt]
}

// Guard against the apply()→router.replace→watch→load() double-fetch loop.
// When apply() pushes new URL params, the watch sees the change and would
// re-call load(); the flag short-circuits one re-entry so we hit the backend
// exactly once per user-initiated Apply.
let suppressNextWatch = false

const severityOptions = [
  { label: 'DEBUG', value: 'DEBUG' },
  { label: 'INFO', value: 'INFO' },
  { label: 'WARN', value: 'WARN' },
  { label: 'ERROR', value: 'ERROR' },
  { label: 'FATAL', value: 'FATAL' },
]

function buildParams() {
  return {
    service: filterService.value ? [filterService.value] : undefined,
    severity: filterSeverity.value.length ? filterSeverity.value : undefined,
    traceId: filterTraceId.value || undefined,
    spanId: filterSpanId.value || undefined,
    bodyContains: filterBodyContains.value || undefined,
    tsFrom: filterTsRange.value ? new Date(filterTsRange.value[0]).toISOString() : undefined,
    tsTo: filterTsRange.value ? new Date(filterTsRange.value[1]).toISOString() : undefined,
    limit: 100,
  }
}

async function apply() {
  // Push filter state into URL query params (source of truth)
  const query: Record<string, string | string[]> = {}
  if (filterService.value) query['service'] = filterService.value
  if (filterSeverity.value.length) query['severity'] = filterSeverity.value
  if (filterTraceId.value) query['trace_id'] = filterTraceId.value
  if (filterSpanId.value) query['span_id'] = filterSpanId.value
  if (filterBodyContains.value) query['body_contains'] = filterBodyContains.value
  if (filterTsRange.value) {
    query['ts_from'] = new Date(filterTsRange.value[0]).toISOString()
    query['ts_to'] = new Date(filterTsRange.value[1]).toISOString()
  }
  suppressNextWatch = true
  await router.replace({ query })
  await load(buildParams())
}

// When URL changes externally (back/forward, cross-jump from another page), reload.
// In-page Apply suppresses this via suppressNextWatch to avoid double-fetch.
watch(
  () => route.query,
  (q) => {
    if (suppressNextWatch) {
      suppressNextWatch = false
      return
    }
    filterService.value = (q['service'] as string) ?? ''
    filterSeverity.value = readQueryArray(q['severity'])
    filterTraceId.value = (q['trace_id'] as string) ?? ''
    filterSpanId.value = (q['span_id'] as string) ?? ''
    filterBodyContains.value = (q['body_contains'] as string) ?? ''
    const f = typeof q['ts_from'] === 'string' ? Date.parse(q['ts_from'] as string) : NaN
    const tt = typeof q['ts_to'] === 'string' ? Date.parse(q['ts_to'] as string) : NaN
    filterTsRange.value = Number.isFinite(f) && Number.isFinite(tt) ? [f, tt] : null
    load(buildParams())
  },
)

onMounted(apply)
watch(refreshTick, apply)
</script>

<template>
  <div class="logs-view" data-testid="logs-page">
    <h1>{{ t('logs.title') }}</h1>

    <NCard>
      <NSpace align="end" wrap>
        <NInput
          v-model:value="filterService"
          :placeholder="t('logs.filter.service')"
          data-testid="filter-service"
          clearable
        />
        <NSelect
          v-model:value="filterSeverity"
          :options="severityOptions"
          :placeholder="t('logs.filter.severity')"
          multiple
          clearable
          style="min-width: 180px"
          data-testid="filter-severity"
        />
        <NDatePicker
          v-model:value="filterTsRange"
          type="datetimerange"
          clearable
          data-testid="filter-ts-range"
        />
        <NInput
          v-model:value="filterTraceId"
          :placeholder="t('logs.filter.traceId')"
          data-testid="filter-trace-id"
          clearable
        />
        <NInput
          v-model:value="filterSpanId"
          :placeholder="t('logs.filter.spanId')"
          data-testid="filter-span-id"
          clearable
        />
        <NInput
          v-model:value="filterBodyContains"
          :placeholder="t('logs.filter.body')"
          data-testid="filter-body"
          clearable
        />
        <NButton type="primary" data-testid="filter-apply" @click="apply">
          {{ t('logs.filter.apply') }}
        </NButton>
      </NSpace>
    </NCard>

    <NAlert v-if="error" type="error" :title="t('logs.errorTitle')" data-testid="logs-error">
      {{ error }}
    </NAlert>

    <NSpin v-if="loading" data-testid="logs-loading" />

    <template v-else>
      <div v-if="items.length === 0" class="logs-empty">
        <NText depth="3" data-testid="logs-empty">{{ t('logs.empty') }}</NText>
      </div>
      <div v-else class="logs-list" data-testid="logs-list">
        <LogRow
          v-for="(log, i) in items"
          :key="`${log.ts}|${log.trace_id}|${log.span_id}|${i}`"
          :log="log"
        />
      </div>
      <NText v-if="hasMore" depth="3" class="has-more">{{ t('logs.hasMore') }}</NText>
    </template>
  </div>
</template>

<style scoped>
.logs-view {
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.logs-list {
  border: 1px solid var(--border, #eee);
  border-radius: 6px;
  overflow: hidden;
}
.logs-empty {
  padding: 24px;
  text-align: center;
}
.has-more {
  padding: 8px 0;
}
</style>
