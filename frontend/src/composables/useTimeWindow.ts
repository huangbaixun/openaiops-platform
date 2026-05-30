import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const ALLOWED = ['15m', '1h', '6h', '24h'] as const
export type TimeWindow = (typeof ALLOWED)[number]

export function isValidWindow(w: unknown): w is TimeWindow {
  return typeof w === 'string' && (ALLOWED as readonly string[]).includes(w)
}

// Module-level auto-refresh state (shared across all useTimeWindow() calls)
const intervalSec = ref(0)
const refreshTick = ref(0)
let _timer: ReturnType<typeof setInterval> | null = null

function setRefreshInterval(sec: number) {
  intervalSec.value = sec
  if (_timer) { clearInterval(_timer); _timer = null }
  if (sec > 0) {
    _timer = setInterval(() => { refreshTick.value++ }, sec * 1000)
  }
}

export function useTimeWindow(defaultWindow: TimeWindow = '1h') {
  // Route/router are only available inside a Vue setup context.
  // useRoute()/useRouter() call inject() which returns undefined outside setup
  // (and emits a Vue warning) without throwing. Guard on the result so that
  // tests accessing the module-level refresh state work without a router.
  const route = useRoute()
  const router = useRouter()

  if (!route || !router) {
    // Outside setup context — refresh-only usage (e.g., setRefreshInterval in tests)
    const windowVal = ref<TimeWindow>(defaultWindow)
    function apply(_next: TimeWindow): void { /* no-op outside setup */ }
    return { windowVal, apply, allowed: ALLOWED, intervalSec, refreshTick, setRefreshInterval }
  }

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

  return { windowVal, apply, allowed: ALLOWED, intervalSec, refreshTick, setRefreshInterval }
}
