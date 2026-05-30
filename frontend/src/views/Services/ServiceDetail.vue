<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { NTabs, NTabPane, NSpin, NAlert } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import TimeWindowPicker from '../../components/TimeWindowPicker.vue'
import SignalsTab from './SignalsTab.vue'
import DependenciesTab from './DependenciesTab.vue'
import ComingSoonTab from './ComingSoonTab.vue'
import { useTimeWindow } from '../../composables/useTimeWindow'
import { fetchServiceDetail, type ServiceDetail as Detail } from '../../api/services'
import AnnotationBadge from '../../components/AnnotationBadge.vue'
import { useAnnotations } from '../../composables/useAnnotations'

const route = useRoute()
const { windowVal } = useTimeWindow()
const { t } = useI18n()
const { annotations: svcAnnotations } = useAnnotations('service', () => String(route.params.name))
const detail = ref<Detail | null>(null)
const loading = ref(false)
const notFound = ref(false)
const error = ref<string | null>(null)

async function load() {
  loading.value = true; error.value = null; notFound.value = false
  try {
    const name = String(route.params.name)
    const r = await fetchServiceDetail(name, windowVal.value)
    if (r === null) { notFound.value = true; detail.value = null } else { detail.value = r }
  } catch (e: any) { error.value = e?.message ?? String(e) }
  finally { loading.value = false }
}
onMounted(load); watch([() => route.params.name, windowVal], load)
</script>
<template>
  <div class="service-detail">
    <div class="section-h">
      <div class="title">
        <h2>{{ route.params.name }}</h2>
        <AnnotationBadge :annotations="svcAnnotations" />
      </div>
      <TimeWindowPicker />
    </div>
    <NAlert v-if="error" type="error">{{ error }}</NAlert>
    <NAlert v-else-if="notFound" type="warning">{{ t('services.notFound') }}</NAlert>
    <NSpin v-else-if="loading || !detail" />
    <NTabs v-else type="line" animated>
      <NTabPane name="signals"      :tab="t('services.tabSignals')"><SignalsTab :detail="detail" /></NTabPane>
      <NTabPane name="deps"         :tab="t('services.tabDependencies')"><DependenciesTab :detail="detail" /></NTabPane>
      <NTabPane name="runtime"      :tab="t('services.tabRuntime')"><ComingSoonTab slice="SLICE-4" /></NTabPane>
      <NTabPane name="exceptions"   :tab="t('services.tabExceptions')"><ComingSoonTab slice="SLICE-4" /></NTabPane>
      <NTabPane name="alerts"       :tab="t('services.tabAlerts')"><ComingSoonTab slice="SLICE-4" /></NTabPane>
      <NTabPane name="settings"     :tab="t('services.tabSettings')"><ComingSoonTab slice="SLICE-5" /></NTabPane>
    </NTabs>
  </div>
</template>
<style scoped>
.service-detail { padding: 24px; }
.title { display: flex; align-items: center; gap: 12px; }
.title h2 { margin: 0; }
</style>
