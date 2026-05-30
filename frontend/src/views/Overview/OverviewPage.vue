<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { NSpin, NAlert } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import TimeWindowPicker from '../../components/TimeWindowPicker.vue'
import ServiceCard from './ServiceCard.vue'
import { useTimeWindow } from '../../composables/useTimeWindow'
import { fetchServices, type ServiceListItem } from '../../api/services'

const router = useRouter()
const { windowVal, refreshTick } = useTimeWindow()
const { t } = useI18n()
const items = ref<ServiceListItem[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

async function load() {
  loading.value = true
  error.value = null
  try { items.value = (await fetchServices(windowVal.value)).items }
  catch (e: any) { error.value = e?.message ?? String(e) }
  finally { loading.value = false }
}
onMounted(load)
watch([windowVal, refreshTick], load)

function go(service: string) { void router.push(`/services/${service}`) }
</script>
<template>
  <div class="overview">
    <div class="section-h"><h3>{{ t('overview.title') }}</h3><TimeWindowPicker /></div>
    <NAlert v-if="error" type="error">{{ error }}</NAlert>
    <NSpin v-else-if="loading" />
    <div v-else class="grid" data-testid="overview-grid">
      <ServiceCard v-for="it in items" :key="it.service" :item="it" @click="go" />
    </div>
  </div>
</template>
<style scoped>
.overview { padding: 4px; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(248px, 1fr)); gap: 14px; }
</style>
