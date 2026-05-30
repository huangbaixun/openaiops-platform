<script setup lang="ts">
import { computed } from 'vue'
const props = defineProps<{ severity: string }>()
const cls = computed(() => {
  const s = props.severity.toUpperCase()
  if (s.includes('ERROR') || s.includes('FATAL')) return 'err'
  if (s.includes('WARN')) return 'warn'
  if (s.includes('INFO')) return 'info'
  return 'muted'
})
</script>
<template><span class="badge" :class="cls">{{ severity }}</span></template>
