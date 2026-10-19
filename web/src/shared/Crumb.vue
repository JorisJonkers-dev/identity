<script setup lang="ts">
import { RouterLink } from 'vue-router'
import Icon from './Icon'

defineProps<{ to: string; label: string }>()
</script>

<template>
  <RouterLink class="crumb" :to="to"><span class="crumb-ic"><Icon name="back" /></span>{{ label }}</RouterLink>
</template>
