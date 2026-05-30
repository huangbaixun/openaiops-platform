<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { NSpin, NAlert } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import TimeWindowPicker from '../../components/TimeWindowPicker.vue'
import ServiceGraph from '../../components/ServiceGraph/ServiceGraph.vue'
import { useTimeWindow } from '../../composables/useTimeWindow'
import { fetchTopology, type TopologyResponse } from '../../api/topology'
import { fetchAnnotations, type Annotation } from '../../api/annotations'

const router = useRouter()
const { windowVal, refreshTick } = useTimeWindow()
const { t } = useI18n()
const data = ref<TopologyResponse>({ window: '1h', nodes: [], edges: [] })
const loading = ref(false)
const error = ref<string | null>(null)
const annByService = ref<Record<string, Annotation[]>>({})

async function load() {
  loading.value = true; error.value = null
  try { data.value = await fetchTopology(windowVal.value, 100) }
  catch (e: any) { error.value = e?.message ?? String(e) }
  finally { loading.value = false }
}
onMounted(load); watch([windowVal, refreshTick], load)

onMounted(async () => {
  try {
    const all = await fetchAnnotations('service')
    annByService.value = all.reduce<Record<string, Annotation[]>>((acc, a) => {
      ;(acc[a.target_id] ??= []).push(a)
      return acc
    }, {})
  } catch { /* badges are non-critical; ignore */ }
})

function go(n: { service: string; kind: string }) {
  if (n.kind === 'external') return
  void router.push(`/services/${n.service}`)
}
</script>
<template>
  <div class="topology">
    <div class="section-h"><h2>{{ t('topology.title') }}</h2><TimeWindowPicker /></div>
    <NAlert v-if="error" type="error">{{ error }}</NAlert>
    <NSpin v-else-if="loading" />
    <div v-else class="card">
      <ServiceGraph :nodes="data.nodes" :edges="data.edges" :ann-by-service="annByService" :width="900" :height="600" @node-click="go" />
    </div>
  </div>
</template>
<style scoped>
.topology { padding: 24px; }
</style>
