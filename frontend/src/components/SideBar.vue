<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
const { t } = useI18n()
const items: { key: string; label: string; to: string | null }[] = [
  { key: 'overview', label: 'nav.overview', to: '/overview' },
  { key: 'services', label: 'nav.services', to: '/overview' },
  { key: 'traces', label: 'nav.traces', to: '/traces' },
  { key: 'logs', label: 'nav.logs', to: '/logs' },
  { key: 'topology', label: 'nav.topology', to: '/topology' },
  { key: 'alerts', label: 'nav.alerts', to: null },
  { key: 'settings', label: 'nav.settings', to: null },
]
</script>

<template>
  <aside>
    <ul>
      <li v-for="it in items" :key="it.key" :class="{ disabled: !it.to }">
        <RouterLink v-if="it.to" :to="it.to" :data-testid="`nav-${it.key}`">
          {{ t(it.label) }}
        </RouterLink>
        <span v-else>{{ t(it.label) }}</span>
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
li { padding: 8px 12px; border-radius: 6px; color: var(--text-3); }
li.disabled { opacity: 0.5; cursor: not-allowed; }
li a, li a:visited { color: var(--text-3); text-decoration: none; display: block; }
li a.router-link-active { color: var(--text-1); }
</style>
