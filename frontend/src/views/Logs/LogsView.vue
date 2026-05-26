<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { NCard, NInput, NSelect, NButton, NSpace, NText, NSpin } from 'naive-ui'
import { useLogsList } from '../../composables/useLogs'
import LogRow from '../../components/LogRow.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const { items, hasMore, loading, load } = useLogsList()

// Filter state (local refs, synced to URL on Apply)
const filterService = ref<string>((route.query['service'] as string) ?? '')
const filterSeverity = ref<string[]>(
  route.query['severity'] ? [route.query['severity'] as string].flat() : [],
)
const filterTraceId = ref<string>((route.query['trace_id'] as string) ?? '')
const filterSpanId = ref<string>((route.query['span_id'] as string) ?? '')
const filterBodyContains = ref<string>((route.query['body_contains'] as string) ?? '')

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
  await router.replace({ query })
  await load(buildParams())
}

// When URL changes externally (e.g. back/forward or cross-jump), sync filter state + reload
watch(
  () => route.query,
  (q) => {
    filterService.value = (q['service'] as string) ?? ''
    filterSeverity.value = q['severity'] ? [q['severity'] as string].flat() : []
    filterTraceId.value = (q['trace_id'] as string) ?? ''
    filterSpanId.value = (q['span_id'] as string) ?? ''
    filterBodyContains.value = (q['body_contains'] as string) ?? ''
    load(buildParams())
  },
)

onMounted(apply)
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

    <NSpin v-if="loading" data-testid="logs-loading" />

    <template v-else>
      <div v-if="items.length === 0" class="logs-empty">
        <NText depth="3" data-testid="logs-empty">{{ t('logs.empty') }}</NText>
      </div>
      <div v-else class="logs-list" data-testid="logs-list">
        <LogRow v-for="log in items" :key="log.ts + log.trace_id + log.span_id" :log="log" />
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
