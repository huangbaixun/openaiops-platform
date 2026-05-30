<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { RouterView } from 'vue-router'
import TopBar from '../components/TopBar.vue'
import SideBar from '../components/SideBar.vue'
import CommandPalette from '../components/CommandPalette.vue'
import { useTheme } from '../composables/useTheme'
import { useSidebar } from '../composables/useSidebar'
import { useCommandPalette } from '../composables/useCommandPalette'

const { initTheme } = useTheme()
const { collapsed, initSidebar } = useSidebar()
const { togglePalette } = useCommandPalette()

function onKeydown(e: KeyboardEvent) {
  if ((e.metaKey || e.ctrlKey) && (e.key === 'k' || e.key === 'K')) {
    e.preventDefault()
    togglePalette()
  }
}

onMounted(() => {
  initTheme()
  initSidebar()
  window.addEventListener('keydown', onKeydown)
})
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <div class="app" :class="{ 'sidebar-collapsed': collapsed }">
    <TopBar />
    <SideBar />
    <main class="main">
      <RouterView />
    </main>
    <CommandPalette />
  </div>
</template>
