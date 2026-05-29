<script setup lang="ts">
import { ref, computed } from 'vue'
import { NTag, NModal, NCard, NList, NListItem, NCode } from 'naive-ui'
import type { Annotation } from '../api/annotations'

const props = defineProps<{ annotations: Annotation[] }>()
const show = ref(false)
const count = computed(() => props.annotations.length)

function pretty(payload: Record<string, unknown>): string {
  return JSON.stringify(payload, null, 2)
}
</script>

<template>
  <span v-if="count > 0">
    <span
      data-testid="annotation-badge"
      style="cursor: pointer; display: inline-flex; align-items: center"
      @click="show = true"
    >
      <NTag
        type="warning"
        size="small"
        round
      >
        🤖 AI · {{ count }}
      </NTag>
    </span>

    <NModal v-model:show="show">
      <NCard
        style="width: 600px; max-width: 90vw"
        title="AI annotations"
        :bordered="false"
        size="huge"
        role="dialog"
        aria-modal="true"
      >
        <NList>
          <NListItem v-for="a in props.annotations" :key="a.id">
            <div>
              <NTag size="tiny" type="info" round>{{ a.kind }}</NTag>
              <span style="margin-left: 8px; opacity: 0.7">{{ a.ts }}</span>
            </div>
            <NCode :code="pretty(a.payload)" language="json" />
          </NListItem>
        </NList>
      </NCard>
    </NModal>
  </span>
</template>
