<script setup lang="ts">
import { RouterView } from 'vue-router'
import TopBar from '../components/TopBar.vue'
import SideBar from '../components/SideBar.vue'
</script>

<template>
  <div class="app">
    <TopBar class="topbar" />
    <SideBar class="sidebar" />
    <main class="main">
      <RouterView />
    </main>
  </div>
</template>

<style scoped>
.app {
  display: grid;
  grid-template-columns: 232px 1fr;
  grid-template-rows: 56px 1fr;
  min-height: 100vh;
}
.topbar { grid-column: 1 / -1; grid-row: 1; }
.sidebar { grid-row: 2; }
.main { grid-row: 2; padding: 20px 24px 60px; overflow: auto; }
</style>
