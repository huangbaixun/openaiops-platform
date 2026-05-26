<script setup lang="ts">
import { computed } from 'vue'
import { NTag } from 'naive-ui'

const props = defineProps<{ severity: string }>()

type TagType = 'default' | 'info' | 'warning' | 'error' | 'success'

const colorMap: Record<string, TagType> = {
  DEBUG: 'default',
  INFO: 'info',
  WARN: 'warning',
  WARNING: 'warning',
  ERROR: 'error',
  FATAL: 'error',
}

const tagType = computed<TagType>(() => colorMap[props.severity.toUpperCase()] ?? 'default')
</script>

<template>
  <NTag :type="tagType" size="small" round>{{ severity }}</NTag>
</template>
