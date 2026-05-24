<script setup lang="ts">
import { NConfigProvider, NMessageProvider } from 'naive-ui'
import { RouterView } from 'vue-router'
</script>

<template>
  <NConfigProvider>
    <NMessageProvider>
      <RouterView />
    </NMessageProvider>
  </NConfigProvider>
</template>
