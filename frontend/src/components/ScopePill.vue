<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '../stores/auth'

const { t } = useI18n()
const auth = useAuthStore()

const open = ref(false)

const activeName = computed(() => {
  const a = auth.domainTenants.find((x) => x.id === auth.activeTenantId)
  return a?.name ?? auth.tenantName ?? '—'
})
const activeEnv = computed(() => {
  const a = auth.domainTenants.find((x) => x.id === auth.activeTenantId)
  return a?.environment || '—'
})
const canSwitch = computed(() => auth.domainTenants.length > 1)

async function pick(id: string) {
  open.value = false
  if (id === auth.activeTenantId) return
  try {
    await auth.switchActiveTenant(id)
    // re-query the current view under the new active tenant
    try { window.location.reload() } catch {}
  } catch {
    // 403 / failure → selection unchanged; surface via title (no toast dep here)
  }
}
</script>

<template>
  <div class="scope-pill">
    <div class="sp-seg" data-testid="scope-domain" :title="t('shell.domainReadonly')">
      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 9l9-7 9 7v11a2 2 0 01-2 2H5a2 2 0 01-2-2z"/></svg>
      <span class="k" style="color: var(--text-3);">Domain</span><span>{{ t('shell.defaultDomain') }}</span>
    </div>
    <span class="sp-sep">/</span>
    <div class="dd" :class="{ open }" style="display:inline-block;">
      <div
        class="sp-seg" data-testid="scope-project"
        :style="{ cursor: canSwitch ? 'pointer' : 'default' }"
        :title="t('shell.projectIsTenant')"
        @click.stop="canSwitch && (open = !open)"
      >
        <span class="k" style="color: var(--text-3);">Project</span>
        <span style="color: var(--accent);">●</span><span>{{ activeName }}</span>
      </div>
      <div v-if="canSwitch" class="dd-menu" style="min-width: 220px;">
        <div class="dd-section">{{ t('shell.switchTenant') }}</div>
        <div
          v-for="opt in auth.domainTenants" :key="opt.id"
          class="dd-item" :class="{ selected: opt.id === auth.activeTenantId }"
          :data-testid="`tenant-opt-${opt.id}`"
          @click="pick(opt.id)"
        >
          <span>{{ opt.name }}</span>
          <span class="kbd">{{ opt.environment || '—' }}</span>
        </div>
      </div>
    </div>
    <span class="sp-sep">/</span>
    <div class="sp-seg" data-testid="scope-env" :title="t('shell.envReadonly')">
      <span class="dot" style="width:6px;height:6px;border-radius:50%;background:var(--success);" />
      <span>{{ activeEnv }}</span>
    </div>
  </div>
</template>

<style scoped>
.sp-sep { color: var(--text-3); padding: 0 2px; }
.sp-seg .k { margin-right: 2px; }
</style>
