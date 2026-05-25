import { ref } from 'vue'
import {
  listTraces,
  getTrace,
  type ListQuery,
  type TraceListItem,
  type TraceDetailResponse,
} from '../api/traces'

export function useTracesList() {
  const items = ref<TraceListItem[]>([])
  const hasMore = ref(false)
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function load(q: ListQuery) {
    loading.value = true
    error.value = null
    try {
      const res = await listTraces(q)
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

export function useTraceDetail() {
  const detail = ref<TraceDetailResponse | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function load(traceId: string) {
    loading.value = true
    error.value = null
    try {
      detail.value = await getTrace(traceId)
    } catch (e) {
      error.value = String(e)
    } finally {
      loading.value = false
    }
  }

  return { detail, loading, error, load }
}
