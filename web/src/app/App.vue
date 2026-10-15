<script setup lang="ts">
import { RouterView } from 'vue-router'
</script>

<template>
  <main class="shell">
    <RouterView />
  </main>
</template>

<style>
body {
  margin: 0;
  font-family: system-ui, sans-serif;
  color: #1a1a1a;
  background: #ffffff;
}
.shell {
  max-width: 40rem;
  margin: 0 auto;
  padding: 1.5rem 1rem;
}
</style>
