<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NCard, NDataTable, NInput, NInputNumber, NButton, NSpace, NText } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { useRouter } from 'vue-router'
import { useTracesList } from '../../composables/useTraces'
import type { TraceListItem } from '../../api/traces'

const { t } = useI18n()
const router = useRouter()
const { items, hasMore, loading, load } = useTracesList()

const service = ref('')
const operation = ref('')
const minDurationMs = ref<number | null>(null)

const columns: DataTableColumns<TraceListItem> = [
  { title: () => t('traces.colTraceId'), key: 'trace_id', ellipsis: { tooltip: true } },
  { title: () => t('traces.colService'), key: 'root_service' },
  { title: () => t('traces.colOperation'), key: 'root_operation', ellipsis: { tooltip: true } },
  { title: () => t('traces.colStart'), key: 'start_ts', sorter: 'default' },
  {
    title: () => t('traces.colDuration'),
    key: 'duration_ns',
    render: (r) => `${(r.duration_ns / 1_000_000).toFixed(2)} ms`,
    sorter: 'default',
  },
  { title: () => t('traces.colSpanCount'), key: 'span_count' },
]

async function apply() {
  await load({
    service: service.value || undefined,
    operation: operation.value || undefined,
    min_duration_ms: minDurationMs.value ?? undefined,
    limit: 100,
    sort: 'ts',
    order: 'desc',
  })
}

function rowProps(row: TraceListItem) {
  return {
    style: 'cursor:pointer',
    onClick: () => router.push(`/traces/${row.trace_id}`),
  }
}

onMounted(apply)
</script>

<template>
  <div class="traces-list" data-testid="traces-page">
    <h1>{{ t('traces.pageTitle') }}</h1>
    <NCard>
      <NSpace align="end">
        <NInput
          v-model:value="service"
          :placeholder="t('traces.filterService')"
          data-testid="filter-service"
        />
        <NInput
          v-model:value="operation"
          :placeholder="t('traces.filterOperation')"
          data-testid="filter-operation"
        />
        <NInputNumber
          v-model:value="minDurationMs"
          :placeholder="t('traces.filterMinDuration')"
          data-testid="filter-min-duration"
        />
        <NButton type="primary" data-testid="filter-apply" @click="apply">
          {{ t('traces.filterApply') }}
        </NButton>
      </NSpace>
    </NCard>
    <NDataTable
      :columns="columns"
      :data="items"
      :loading="loading"
      :row-props="rowProps"
      :bordered="false"
      data-testid="traces-table"
    />
    <NText v-if="items.length === 0 && !loading" depth="3">
      {{ t('traces.empty') }}
    </NText>
    <NText v-if="hasMore" depth="3">{{ t('traces.hasMore') }}</NText>
  </div>
</template>

<style scoped>
.traces-list { padding: 24px; display: flex; flex-direction: column; gap: 16px; }
</style>
