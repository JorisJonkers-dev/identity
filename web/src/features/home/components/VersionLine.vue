<script setup lang="ts">
import { useVersion } from '../composables/useVersion'

const { version, loadError } = useVersion()
</script>

<!-- The build that is serving, at the foot of the home page. -->
<template>
  <p v-if="loadError" role="alert" class="alert">{{ loadError }}</p>
  <footer v-else-if="version" class="text-[12px] text-ink-3">Version {{ version }}</footer>
</template>
