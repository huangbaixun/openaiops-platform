<script setup lang="ts">
import { NEmpty } from 'naive-ui'
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
const props = defineProps<{ slice: string }>()
</script>
<template>
  <NEmpty :description="t('services.comingSoon', { slice: props.slice })" :data-testid="`coming-soon-${props.slice.toLowerCase()}`" />
</template>
