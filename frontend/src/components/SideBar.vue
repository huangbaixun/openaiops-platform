<script setup lang="ts">
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
const items = [
  { key: 'overview', label: 'nav.overview' },
  { key: 'services', label: 'nav.services' },
  { key: 'traces', label: 'nav.traces' },
  { key: 'logs', label: 'nav.logs' },
  { key: 'topology', label: 'nav.topology' },
  { key: 'alerts', label: 'nav.alerts' },
  { key: 'settings', label: 'nav.settings' },
]
</script>

<template>
  <aside>
    <ul>
      <li v-for="it in items" :key="it.key" class="disabled" :title="'Slice 0 placeholder'">
        {{ t(it.label) }}
      </li>
    </ul>
  </aside>
</template>

<style scoped>
aside {
  background: var(--bg-sidebar);
  border-right: 1px solid var(--border);
  backdrop-filter: blur(20px);
}
ul { list-style: none; margin: 0; padding: 12px 8px; }
li { padding: 8px 12px; border-radius: 6px; color: var(--text-3); cursor: not-allowed; }
li.disabled { opacity: 0.5; }
</style>
