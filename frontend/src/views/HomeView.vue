<script setup lang="ts">
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
</script>

<template>
  <div class="home">
    <h1>{{ t('home.welcome') }}</h1>
    <p>{{ t('home.placeholder') }}</p>
  </div>
</template>

<style scoped>
.home { padding: 24px; }
h1 { color: var(--text-1); }
p { color: var(--text-2); }
</style>
