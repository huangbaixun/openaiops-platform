<script setup lang="ts">
import { ref, computed } from 'vue'
import { RouterLink } from 'vue-router'
import { NText } from 'naive-ui'
import SeverityBadge from './SeverityBadge.vue'
import type { LogItem } from '../api/logs'

const props = defineProps<{ log: LogItem }>()

const expanded = ref(false)

const BODY_TRUNCATE = 200

const bodyPreview = computed(() =>
  props.log.body.length > BODY_TRUNCATE
    ? props.log.body.slice(0, BODY_TRUNCATE) + '…'
    : props.log.body,
)

const bodyFormatted = computed(() => {
  try {
    return JSON.stringify(JSON.parse(props.log.body), null, 2)
  } catch {
    return props.log.body
  }
})

const traceLink = computed(() => {
  if (!props.log.trace_id) return null
  const base = `/traces/${props.log.trace_id}`
  return props.log.span_id ? `${base}?focus_span=${props.log.span_id}` : base
})

const rowTestId = computed(() =>
  props.log.trace_id ? `log-row-${props.log.trace_id}` : `log-row-${props.log.ts}`,
)

function toggle() {
  expanded.value = !expanded.value
}
</script>

<template>
  <div
    class="log-row"
    :data-testid="rowTestId"
    @click="toggle"
  >
    <!-- Compact header row -->
    <div class="log-row-head">
      <NText depth="3" class="log-ts">{{ log.ts }}</NText>
      <SeverityBadge :severity="log.severity_text" />
      <NText class="log-service">{{ log.service }}</NText>
      <NText class="log-body-preview">{{ bodyPreview }}</NText>
      <RouterLink
        v-if="traceLink"
        :to="traceLink"
        class="trace-chip"
        :data-testid="`trace-link-${log.trace_id}`"
        @click.stop
      >
        {{ log.trace_id.slice(0, 16) }}
      </RouterLink>
    </div>

    <!-- Expanded detail -->
    <div v-if="expanded" class="log-row-detail" @click.stop>
      <div class="log-detail-section">
        <NText strong>Body</NText>
        <pre class="log-body-full">{{ bodyFormatted }}</pre>
      </div>
      <div v-if="Object.keys(log.attributes).length" class="log-detail-section">
        <NText strong>Attributes</NText>
        <pre class="log-attrs">{{ JSON.stringify(log.attributes, null, 2) }}</pre>
      </div>
      <div v-if="Object.keys(log.resource_attributes).length" class="log-detail-section">
        <NText strong>Resource Attributes</NText>
        <pre class="log-attrs">{{ JSON.stringify(log.resource_attributes, null, 2) }}</pre>
      </div>
      <div v-if="log.span_id" class="log-detail-section">
        <NText depth="3">span_id: {{ log.span_id }}</NText>
      </div>
    </div>
  </div>
</template>

<style scoped>
.log-row {
  padding: 8px 12px;
  background: var(--bg-elev-0, #fff);
  border: 1px solid var(--border, #eee);
  border-radius: 6px;
  margin-bottom: 4px;
  cursor: pointer;
  transition: background 0.15s;
}
.log-row:hover {
  background: var(--bg-hover, rgba(0, 0, 0, 0.03));
}
.log-row-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.log-ts {
  font-size: 12px;
  white-space: nowrap;
  flex-shrink: 0;
}
.log-service {
  font-weight: 500;
  flex-shrink: 0;
}
.log-body-preview {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
.trace-chip {
  font-size: 11px;
  font-family: monospace;
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--bg-chip, rgba(24, 144, 255, 0.1));
  color: var(--color-primary, #1890ff);
  text-decoration: none;
  white-space: nowrap;
  flex-shrink: 0;
}
.trace-chip:hover {
  text-decoration: underline;
}
.log-row-detail {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px dashed var(--border, #eee);
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.log-detail-section {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.log-body-full,
.log-attrs {
  background: var(--bg-code, #f5f5f5);
  padding: 8px;
  border-radius: 4px;
  font-size: 12px;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
}
</style>
