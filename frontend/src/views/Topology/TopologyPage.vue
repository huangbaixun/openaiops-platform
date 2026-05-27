<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { NSpin, NAlert } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import TimeWindowPicker from '../../components/TimeWindowPicker.vue'
import ServiceGraph from '../../components/ServiceGraph/ServiceGraph.vue'
import { useTimeWindow } from '../../composables/useTimeWindow'
import { fetchTopology, type TopologyResponse } from '../../api/topology'

const router = useRouter()
const { windowVal } = useTimeWindow()
const { t } = useI18n()
const data = ref<TopologyResponse>({ window: '1h', nodes: [], edges: [] })
const loading = ref(false)
const error = ref<string | null>(null)

async function load() {
  loading.value = true; error.value = null
  try { data.value = await fetchTopology(windowVal.value, 100) }
  catch (e: any) { error.value = e?.message ?? String(e) }
  finally { loading.value = false }
}
onMounted(load); watch(windowVal, load)

function go(n: { service: string; kind: string }) {
  if (n.kind === 'external') return
  void router.push(`/services/${n.service}`)
}
</script>
<template>
  <div class="topology">
    <div class="header"><h2>{{ t('topology.title') }}</h2><TimeWindowPicker /></div>
    <NAlert v-if="error" type="error">{{ error }}</NAlert>
    <NSpin v-else-if="loading" />
    <ServiceGraph v-else :nodes="data.nodes" :edges="data.edges" :width="900" :height="600" @node-click="go" />
  </div>
</template>
<style scoped>
.topology { padding: 24px; }
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
</style>
