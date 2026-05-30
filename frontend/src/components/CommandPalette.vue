<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import { fetchServices, type ServiceListItem } from '../api/services'
import { useTimeWindow } from '../composables/useTimeWindow'
import { buildItems, fuzzyFilter, useCommandPalette } from '../composables/useCommandPalette'

const router = useRouter()
const { windowVal } = useTimeWindow()
const { open, closePalette } = useCommandPalette()

const query = ref('')
const index = ref(0)
const services = ref<ServiceListItem[]>([])
const loadError = ref(false)

function navigate(path: string) { closePalette(); void router.push(path) }
const allItems = computed(() => buildItems(services.value, navigate))
const results = computed(() => fuzzyFilter(allItems.value, query.value))

async function loadServices() {
  try { services.value = (await fetchServices(windowVal.value)).items; loadError.value = false }
  catch { services.value = []; loadError.value = true }
}

watch(open, (isOpen) => {
  if (isOpen) { query.value = ''; index.value = 0; void loadServices() }
})

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'ArrowDown') { e.preventDefault(); index.value = Math.min(results.value.length - 1, index.value + 1) }
  else if (e.key === 'ArrowUp') { e.preventDefault(); index.value = Math.max(0, index.value - 1) }
  else if (e.key === 'Enter') { e.preventDefault(); results.value[index.value]?.action() }
  else if (e.key === 'Escape') { closePalette() }
}
</script>

<template>
  <div v-if="open" id="cmdk-root" data-testid="command-palette">
    <div class="cmdk-bg" @click="closePalette" />
    <div class="cmdk" role="dialog" aria-modal="true" aria-label="Command palette">
      <div class="cmdk-input-row">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="7"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
        <input
          autofocus autocomplete="off" spellcheck="false"
          placeholder="搜索服务、页面…"
          :value="query"
          data-testid="cmdk-input"
          @input="(e) => { query = (e.target as HTMLInputElement).value; index = 0 }"
          @keydown="onKeydown"
        >
        <span class="kbd">ESC</span>
      </div>
      <div class="cmdk-list">
        <div v-if="loadError" class="cmdk-empty">服务列表加载失败 · 仍可跳转页面</div>
        <div v-if="results.length === 0" class="cmdk-empty">无匹配项</div>
        <div
          v-for="(it, i) in results" :key="it.type + it.label"
          class="cmdk-item" :class="{ active: i === index }"
          :data-testid="`cmdk-item-${it.label}`"
          @mousemove="index = i"
          @click="it.action()"
        >
          <span class="cmdk-ic">{{ it.type === 'service' ? '⊙' : '→' }}</span>
          <span class="cmdk-label">{{ it.label }}</span>
          <span class="cmdk-hint">{{ it.hint }}</span>
        </div>
      </div>
      <div class="cmdk-foot">
        <span><span class="kbd">↑↓</span>移动</span>
        <span><span class="kbd">↵</span>选择</span>
        <span><span class="kbd">⌘K</span>开关</span>
      </div>
    </div>
  </div>
</template>
