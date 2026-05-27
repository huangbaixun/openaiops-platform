<script setup lang="ts">
import { NRadioGroup, NRadioButton } from 'naive-ui'
import { useTimeWindow, type TimeWindow } from '../composables/useTimeWindow'
import { useI18n } from 'vue-i18n'

const { windowVal, apply, allowed } = useTimeWindow()
const { t } = useI18n()
function onUpdate(v: string) {
  apply(v as TimeWindow)
}
</script>
<template>
  <NRadioGroup
    :value="windowVal"
    @update:value="onUpdate"
    data-testid="time-window-picker"
    size="small"
  >
    <NRadioButton
      v-for="w in allowed"
      :key="w"
      :value="w"
      :data-testid="`window-${w}`"
    >
      {{ t(`timeWindow.${w}`) }}
    </NRadioButton>
  </NRadioGroup>
</template>
