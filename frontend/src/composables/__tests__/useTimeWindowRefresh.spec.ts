import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { useTimeWindow } from '../useTimeWindow'

describe('useTimeWindow auto-refresh', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => { useTimeWindow().setRefreshInterval(0); vi.useRealTimers() })

  it('exposes intervalSec defaulting to 0 (off)', () => {
    expect(useTimeWindow().intervalSec.value).toBe(0)
  })

  it('setRefreshInterval bumps refreshTick on each interval', () => {
    const { intervalSec, refreshTick, setRefreshInterval } = useTimeWindow()
    const before = refreshTick.value
    setRefreshInterval(30)
    expect(intervalSec.value).toBe(30)
    vi.advanceTimersByTime(60_000)
    expect(refreshTick.value).toBe(before + 2)
  })
})
