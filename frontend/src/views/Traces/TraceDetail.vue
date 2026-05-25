<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NTabs, NTabPane, NEmpty, NSpin } from 'naive-ui'
import WaterfallChart from './WaterfallChart.vue'
import { useTraceDetail } from '../../composables/useTraces'

const props = defineProps<{ traceId: string }>()
const { t } = useI18n()
const { detail, loading, load } = useTraceDetail()
const active = ref<'waterfall' | 'json' | 'serviceMap'>('waterfall')

onMounted(() => load(props.traceId))
watch(() => props.traceId, (id) => load(id))
</script>

<template>
  <div class="trace-detail" data-testid="trace-detail-page">
    <NSpin :show="loading">
      <NTabs v-model:value="active" type="line" animated>
        <NTabPane name="waterfall" :tab="t('traces.tabWaterfall')">
          <WaterfallChart v-if="detail" :spans="detail.spans" />
          <NEmpty v-else />
        </NTabPane>

        <NTabPane name="json" :tab="t('traces.tabJSON')">
          <pre v-if="detail" class="trace-json" data-testid="trace-json">{{
            JSON.stringify(detail, null, 2)
          }}</pre>
          <NEmpty v-else />
        </NTabPane>

        <NTabPane name="serviceMap" :tab="t('traces.tabServiceMap')">
          <NEmpty
            :description="t('traces.serviceMapComingSoon')"
            data-testid="service-map-placeholder"
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
