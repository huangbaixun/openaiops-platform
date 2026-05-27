<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NTabs, NTabPane, NEmpty, NSpin } from 'naive-ui'
import WaterfallChart from './WaterfallChart.vue'
import ServiceMapPanel from './ServiceMapPanel.vue'
import LogsPanel from '../../components/LogsPanel.vue'
import { useTraceDetail } from '../../composables/useTraces'
import type { GraphNode } from '../../components/ServiceGraph/types'

const props = defineProps<{ traceId: string }>()
const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const { detail, loading, load } = useTraceDetail()
const active = ref<'waterfall' | 'json' | 'serviceMap' | 'logs'>('waterfall')
const selectedSpanId = ref<string | null>(null)

onMounted(() => {
  load(props.traceId)
  const focusSpan = route.query.focus_span
  if (focusSpan && typeof focusSpan === 'string') {
    selectedSpanId.value = focusSpan
  }
})
watch(() => props.traceId, (id) => load(id))

function onSpanClick(spanId: string) {
  selectedSpanId.value = spanId
}

function clearSpan() {
  selectedSpanId.value = null
}

function onServiceMapClick(n: GraphNode) {
  // V1: set ?focus_service=X so Waterfall can scroll. Waterfall integration is post-MVP.
  const q = { ...route.query, focus_service: n.service }
  void router.replace({ query: q })
}
</script>

<template>
  <div class="trace-detail" data-testid="trace-detail-page">
    <NSpin :show="loading">
      <NTabs v-model:value="active" type="line" animated>
        <NTabPane name="waterfall" :tab="t('traces.tabWaterfall')">
          <WaterfallChart v-if="detail" :spans="detail.spans" @span-click="onSpanClick" />
          <NEmpty v-else />
        </NTabPane>

        <NTabPane name="json" :tab="t('traces.tabJSON')">
          <pre v-if="detail" class="trace-json" data-testid="trace-json">{{
            JSON.stringify(detail, null, 2)
          }}</pre>
          <NEmpty v-else />
        </NTabPane>

        <NTabPane name="serviceMap" :tab="t('traces.tabServiceMap')">
          <ServiceMapPanel
            v-if="detail"
            :spans="detail.spans"
            @node-click="onServiceMapClick"
          />
          <NEmpty v-else />
        </NTabPane>

        <NTabPane name="logs" :tab="t('logs.tab')">
          <LogsPanel
            :trace-id="props.traceId"
            :span-id="selectedSpanId"
            @clear-span="clearSpan"
          />
        </NTabPane>
      </NTabs>
    </NSpin>
  </div>
</template>

<style scoped>
.trace-detail { padding: 24px; }
.trace-json {
  background: #0b0f15;
  color: #cbd5e1;
  padding: 12px;
  border-radius: 6px;
  overflow: auto;
  max-height: 70vh;
  font-size: 12px;
}
</style>
