// frontend/src/composables/useTheme.ts
import { ref } from 'vue'

export type Theme = 'light' | 'dark'
const KEY = 'apm:theme'
const theme = ref<Theme>('light')

function apply(t: Theme) {
  theme.value = t
  document.documentElement.setAttribute('data-theme', t)
  try { localStorage.setItem(KEY, t) } catch { /* Storage shim; ignore */ }
}

export function useTheme() {
  function initTheme() {
    let saved: Theme = 'light'
    try {
      const v = localStorage.getItem(KEY)
      if (v === 'dark' || v === 'light') saved = v
    } catch { /* ignore */ }
    apply(saved)
  }
  function toggleTheme() {
    apply(theme.value === 'dark' ? 'light' : 'dark')
  }
  return { theme, initTheme, toggleTheme }
}
