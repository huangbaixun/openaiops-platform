<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { navIcons } from './NavIcons'
import { useSidebar } from '../composables/useSidebar'

const { t } = useI18n()
const { collapsed, toggleSidebar } = useSidebar()

type NavItem = { id: string; label: string; to: string | null; badge?: string }
const groups: { group: string; items: NavItem[] }[] = [
  { group: 'OBSERVE', items: [
    { id: 'overview', label: 'nav.overview', to: '/overview' },
    { id: 'traces', label: 'nav.traces', to: '/traces' },
    { id: 'logs', label: 'nav.logs', to: '/logs' },
    { id: 'exceptions', label: 'nav.exceptions', to: null },
  ]},
  { group: 'ANALYZE', items: [
    { id: 'topology', label: 'nav.topology', to: '/topology' },
    { id: 'database', label: 'nav.database', to: null },
    { id: 'redis', label: 'nav.redis', to: null },
    { id: 'kafka', label: 'nav.kafka', to: null, badge: 'NEW' },
    { id: 'llm', label: 'nav.llm', to: null, badge: 'NEW' },
  ]},
  { group: 'PLATFORM', items: [
    { id: 'dashboards', label: 'nav.dashboards', to: null },
    { id: 'alerts', label: 'nav.alerts', to: null },
    { id: 'onboarding', label: 'nav.onboarding', to: null },
    { id: 'settings', label: 'nav.settings', to: null },
  ]},
]
const collapseLabel = computed(() => (collapsed.value ? t('shell.expand') : t('shell.collapse')))
</script>

<template>
  <aside class="sidebar">
    <div class="sidebar-toggle" :title="collapseLabel" @click="toggleSidebar">
      <svg class="ic-collapse" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>
      <span class="lbl">{{ t('shell.collapse') }}</span>
    </div>
    <div v-for="g in groups" :key="g.group" class="nav-group">
      <div class="label">{{ g.group }}</div>
      <template v-for="it in g.items" :key="it.id">
        <RouterLink
          v-if="it.to"
          class="nav-item"
          active-class="active"
          :to="it.to"
          :data-testid="`nav-${it.id}`"
          :title="t(it.label)"
        >
          <span class="ico" v-html="navIcons[it.id]" />
          <span class="lbl">{{ t(it.label) }}</span>
          <span v-if="it.badge" class="badge-mini" style="background: var(--accent); color: white;">{{ it.badge }}</span>
        </RouterLink>
        <div
          v-else
          class="nav-item disabled"
          :data-testid="`nav-${it.id}`"
          :title="t('shell.comingSoon')"
        >
          <span class="ico" v-html="navIcons[it.id]" />
          <span class="lbl">{{ t(it.label) }}</span>
          <span class="badge-mini" style="background: var(--bg-hover); color: var(--text-3);">{{ it.badge || t('shell.soon') }}</span>
        </div>
      </template>
    </div>
  </aside>
</template>

<style scoped>
.nav-item.disabled { opacity: .55; cursor: not-allowed; }
.nav-item.disabled:hover { background: transparent; color: var(--text-2); }
.sidebar :deep(.ico svg) { width: 16px; height: 16px; }
</style>
