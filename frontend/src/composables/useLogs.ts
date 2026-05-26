import { ref } from 'vue'
import { listLogs, type LogsListParams, type LogItem } from '../api/logs'

export function useLogsList() {
  const items = ref<LogItem[]>([])
  const hasMore = ref(false)
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function load(params: LogsListParams) {
    loading.value = true
    error.value = null
    try {
      const res = await listLogs(params)
      items.value = res.items
      hasMore.value = res.has_more
    } catch (e) {
      error.value = String(e)
    } finally {
      loading.value = false
    }
  }

  return { items, hasMore, loading, error, load }
}
