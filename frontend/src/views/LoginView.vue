<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NCard, NInput, NButton, NSelect, useMessage } from 'naive-ui'
import { useAuthStore } from '../stores/auth'
import { setLocale } from '../i18n'

const { t, locale } = useI18n()
const router = useRouter()
const message = useMessage()
const auth = useAuthStore()

const apiKey = ref('')
const loading = ref(false)

async function submit() {
  loading.value = true
  try {
    await auth.login(apiKey.value)
    router.push({ name: 'home' })
  } catch {
    message.error(t('login.error'))
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <NCard :title="t('login.title')" class="login-card">
      <div class="row">
        <label>{{ t('login.apiKey') }}</label>
        <NInput
          v-model:value="apiKey"
          type="password"
          show-password-on="click"
          :placeholder="t('login.apiKeyPlaceholder')"
          @keyup.enter="submit"
          data-testid="apiKey-input"
        />
      </div>
      <div class="row actions">
        <NButton type="primary" :loading="loading" @click="submit" data-testid="submit-btn">
          {{ t('login.submit') }}
        </NButton>
      </div>
      <div class="row lang">
        <NSelect
          :value="locale"
          :options="[
            { label: '中文', value: 'zh-CN' },
            { label: 'English', value: 'en-US' },
          ]"
          @update:value="(v) => setLocale(v as 'zh-CN' | 'en-US')"
          data-testid="lang-select"
        />
      </div>
    </NCard>
  </div>
</template>

<style scoped>
.login-page {
  display: grid;
  place-items: center;
  min-height: 100vh;
  background: var(--bg);
}
.login-card {
  width: 360px;
  background: var(--bg-elev-2);
  box-shadow: var(--shadow-md);
}
.row { margin-bottom: 12px; }
.row label { display: block; margin-bottom: 6px; color: var(--text-2); }
.row.actions { display: flex; justify-content: flex-end; }
.row.lang { display: flex; justify-content: flex-end; }
</style>
