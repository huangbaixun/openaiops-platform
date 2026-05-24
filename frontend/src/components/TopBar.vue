<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { NButton, NSelect } from 'naive-ui'
import { useAuthStore } from '../stores/auth'
import { setLocale } from '../i18n'

const { t, locale } = useI18n()
const auth = useAuthStore()
const router = useRouter()

function logout() {
  auth.logout()
  router.push({ name: 'login' })
}
</script>

<template>
  <header>
    <span class="brand">OpenAIOps Platform</span>
    <span class="spacer" />
    <span class="tenant" data-testid="tenant-name">
      {{ t('topbar.tenant') }}: {{ auth.tenantName }}
    </span>
    <NSelect
      class="lang"
      :value="locale"
      :options="[
        { label: '中', value: 'zh-CN' },
        { label: 'EN', value: 'en-US' },
      ]"
      @update:value="(v) => setLocale(v as 'zh-CN' | 'en-US')"
    />
    <NButton size="small" @click="logout">{{ t('topbar.logout') }}</NButton>
  </header>
</template>

<style scoped>
header {
  display: flex;
  align-items: center;
  gap: 12px;
  height: 56px;
  padding: 0 16px;
  background: var(--bg-topbar);
  border-bottom: 1px solid var(--border);
  backdrop-filter: blur(20px);
}
.brand { font-weight: 600; color: var(--text-1); }
.spacer { flex: 1; }
.tenant { color: var(--text-2); }
.lang { width: 70px; }
</style>
