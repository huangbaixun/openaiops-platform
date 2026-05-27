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
const { windowVal } = useTimeWindow()
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
watch(windowVal, load)

function go(service: string) { void router.push(`/services/${service}`) }
</script>
<template>
  <div class="overview">
    <div class="header"><h2>{{ t('overview.title') }}</h2><TimeWindowPicker /></div>
    <NAlert v-if="error" type="error">{{ error }}</NAlert>
    <NSpin v-else-if="loading" />
    <div v-else class="grid" data-testid="overview-grid">
      <ServiceCard v-for="it in items" :key="it.service" :item="it" @click="go" />
    </div>
  </div>
</template>
<style scoped>
.overview { padding: 24px; }
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 16px; }
</style>
