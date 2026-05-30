import { ref } from 'vue'

const KEY = 'apm:sidebar'
const collapsed = ref(false)

export function useSidebar() {
  function initSidebar() {
    try { collapsed.value = localStorage.getItem(KEY) === '1' } catch { collapsed.value = false }
  }
  function toggleSidebar() {
    collapsed.value = !collapsed.value
    try { localStorage.setItem(KEY, collapsed.value ? '1' : '0') } catch { /* ignore */ }
  }
  return { collapsed, initSidebar, toggleSidebar }
}
