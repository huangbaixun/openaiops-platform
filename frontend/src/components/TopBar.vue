<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { NSelect } from 'naive-ui'
import { useAuthStore } from '../stores/auth'
import { setLocale } from '../i18n'
import { useTheme } from '../composables/useTheme'
import { useTimeWindow, type TimeWindow } from '../composables/useTimeWindow'
import ScopePill from './ScopePill.vue'

const { t, locale } = useI18n()
const auth = useAuthStore()
const router = useRouter()
const { toggleTheme } = useTheme()
const { windowVal, apply, allowed, intervalSec, setRefreshInterval } = useTimeWindow()

const userMenuOpen = ref(false)
const timeOpen = ref(false)
const windowLabels: Record<TimeWindow, string> = { '15m': 'Last 15m', '1h': 'Last 1h', '6h': 'Last 6h', '24h': 'Last 24h' }

function logout() { auth.logout(); void router.push({ name: 'login' }) }
const initials = () => (auth.tenantName || '?').slice(0, 2).toUpperCase()
</script>

<template>
  <header class="topbar">
    <div class="brand">
      <div class="logo">
        <svg width="18" height="18" viewBox="0 0 32 32" fill="none"><g fill="#fff"><circle cx="14.6" cy="11.2" r="4.8"/><path d="M11.6 14 C 8.6 16, 8 20, 10 24 C 12 27, 16 27, 18 24.5 C 20 22, 19 18, 16.8 15.6 Z"/><path d="M18.4 10.6 L29 11.8 L18.4 13.6 Z"/></g></svg>
      </div>
      <div><div class="name">OpenAIOps <span class="sub">APM</span></div></div>
    </div>

    <div class="filters">
      <ScopePill />
    </div>

    <div class="spacer" />

    <div class="right">
      <div class="dd" :class="{ open: timeOpen }">
        <button class="pill" data-testid="time-range" @click.stop="timeOpen = !timeOpen">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="9"/><polyline points="12 7 12 12 15 14"/></svg>
          <span class="k">时间</span><span class="v">{{ windowLabels[windowVal] }}</span>
        </button>
        <div class="dd-menu">
          <div
            v-for="w in allowed" :key="w"
            class="dd-item" :class="{ selected: w === windowVal }"
            :data-testid="`time-${w}`"
            @click="apply(w as TimeWindow); timeOpen = false"
          >
            <span>{{ windowLabels[w as TimeWindow] }}</span><span class="kbd">{{ w }}</span>
          </div>
        </div>
      </div>

      <div class="refresh-group" :title="t('shell.autoRefresh')">
        <select :value="intervalSec" data-testid="auto-refresh" @change="(e) => setRefreshInterval(Number((e.target as HTMLSelectElement).value))">
          <option :value="0">关闭</option>
          <option :value="30">30s</option>
          <option :value="60">1m</option>
          <option :value="300">5m</option>
        </select>
      </div>

      <button class="icon-btn theme-btn" data-testid="theme-toggle" :title="t('shell.theme')" @click="toggleTheme">
        <svg class="theme-moon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1111.21 3 7 7 0 0021 12.79z"/></svg>
        <svg class="theme-sun" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41"/></svg>
      </button>

      <NSelect
        class="lang" size="small" :value="locale" data-testid="lang-select"
        :options="[{ label: '中', value: 'zh-CN' }, { label: 'EN', value: 'en-US' }]"
        style="width: 72px;"
        @update:value="(v) => setLocale(v as 'zh-CN' | 'en-US')"
      />

      <div class="dd" :class="{ open: userMenuOpen }">
        <div class="avatar" data-testid="user-avatar" style="cursor: pointer;" @click.stop="userMenuOpen = !userMenuOpen">{{ initials() }}</div>
        <div class="dd-menu" style="left: auto; right: 0;">
          <div class="dd-section" data-testid="tenant-name">{{ t('topbar.tenant') }}: {{ auth.tenantName }}</div>
          <div class="dd-sep" />
          <div class="dd-item" data-testid="logout-btn" @click="logout">
            <span style="color: var(--danger);">{{ t('shell.logout') }}</span>
          </div>
        </div>
      </div>
    </div>
  </header>
</template>

<style scoped>
.topbar :deep(.lang .n-base-selection) { min-height: 32px; }
</style>
