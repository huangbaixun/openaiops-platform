// frontend/src/composables/__tests__/useTheme.spec.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { useTheme } from '../useTheme'

describe('useTheme', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.removeAttribute('data-theme')
  })

  it('defaults to light and reflects on <html>', () => {
    const { theme, initTheme } = useTheme()
    initTheme()
    expect(theme.value).toBe('light')
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
  })

  it('toggle flips theme, sets data-theme, and persists', () => {
    const { theme, initTheme, toggleTheme } = useTheme()
    initTheme()
    toggleTheme()
    expect(theme.value).toBe('dark')
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
    expect(localStorage.getItem('apm:theme')).toBe('dark')
  })

  it('restores persisted theme on init', () => {
    localStorage.setItem('apm:theme', 'dark')
    const { theme, initTheme } = useTheme()
    initTheme()
    expect(theme.value).toBe('dark')
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
  })
})
