import { describe, it, expect, beforeEach } from 'vitest'
import { useSidebar } from '../useSidebar'

describe('useSidebar', () => {
  beforeEach(() => localStorage.clear())

  it('defaults to expanded', () => {
    const { collapsed, initSidebar } = useSidebar()
    initSidebar()
    expect(collapsed.value).toBe(false)
  })

  it('toggle flips and persists', () => {
    const { collapsed, initSidebar, toggleSidebar } = useSidebar()
    initSidebar()
    toggleSidebar()
    expect(collapsed.value).toBe(true)
    expect(localStorage.getItem('apm:sidebar')).toBe('1')
  })

  it('restores persisted collapsed state', () => {
    localStorage.setItem('apm:sidebar', '1')
    const { collapsed, initSidebar } = useSidebar()
    initSidebar()
    expect(collapsed.value).toBe(true)
  })
})
