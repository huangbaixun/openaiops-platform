<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '../stores/auth'

// FEATURE-B (PLATFORM-MT-1): wire real tenant switching here.
// Today: Project = the single logged-in tenant; Domain/Env are read-only labels
// derived statically. Feature B replaces these constants with real lists +
// switch handlers and turns the segments into active dropdowns.
const { t } = useI18n()
const auth = useAuthStore()
const project = computed(() => auth.tenantName || '—')
const DOMAIN_LABEL = computed(() => t('shell.defaultDomain')) // static until Feature B
const ENV_LABEL = 'Production' // static until Feature B
</script>

<template>
  <div class="scope-pill">
    <div class="sp-seg" data-testid="scope-domain" :title="t('shell.domainReadonly')">
      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 9l9-7 9 7v11a2 2 0 01-2 2H5a2 2 0 01-2-2z"/></svg>
      <span class="k" style="color: var(--text-3);">Domain</span><span>{{ DOMAIN_LABEL }}</span>
    </div>
    <span class="sp-sep">/</span>
    <div class="sp-seg" data-testid="scope-project" :title="t('shell.projectIsTenant')">
      <span class="k" style="color: var(--text-3);">Project</span>
      <span style="color: var(--accent);">●</span><span>{{ project }}</span>
    </div>
    <span class="sp-sep">/</span>
    <div class="sp-seg" data-testid="scope-env" :title="t('shell.envReadonly')">
      <span class="dot" style="width:6px;height:6px;border-radius:50%;background:var(--success);" />
      <span>{{ ENV_LABEL }}</span>
    </div>
  </div>
</template>

<style scoped>
.sp-sep { color: var(--text-3); padding: 0 2px; }
.sp-seg { cursor: default; }
.sp-seg .k { margin-right: 2px; }
</style>
