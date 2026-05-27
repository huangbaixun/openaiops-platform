import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const ALLOWED = ['15m', '1h', '6h', '24h'] as const
export type TimeWindow = (typeof ALLOWED)[number]

export function isValidWindow(w: unknown): w is TimeWindow {
  return typeof w === 'string' && (ALLOWED as readonly string[]).includes(w)
}

export function useTimeWindow(defaultWindow: TimeWindow = '1h') {
  const route = useRoute()
  const router = useRouter()
  const initial = isValidWindow(route.query.window)
    ? (route.query.window as TimeWindow)
    : defaultWindow
  const windowVal = ref<TimeWindow>(initial)
  let suppressNextWatch = false

  function apply(next: TimeWindow): void {
    if (next === windowVal.value) return
    suppressNextWatch = true
    void router.replace({ query: { ...route.query, window: next } })
    windowVal.value = next
  }

  watch(
    () => route.query.window,
    (q) => {
      if (suppressNextWatch) {
        suppressNextWatch = false
        return
      }
      windowVal.value = isValidWindow(q) ? (q as TimeWindow) : defaultWindow
    },
  )

  return { windowVal, apply, allowed: ALLOWED }
}
