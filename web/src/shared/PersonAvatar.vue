<script setup lang="ts">
import { computed } from 'vue'
import { initials } from './initials'

const props = defineProps<{ name: string; size?: 'xs' | 'sm' | 'lg' | 'xl' | 'xxl' }>()
// Each person keeps one colour, picked from their name.
const hues = ['av-al', 'av-jj', 'av-fb', 'av-nb', 'av-lb', 'av-db', 'av-pb', 'av-rv', 'av-im', 'av-tg', 'av-mj', 'av-sv']
const tone = computed(() => hues[Array.from(props.name).reduce((n, c) => n + c.charCodeAt(0), 0) % hues.length])
</script>

<template>
  <span :class="['av', size, tone]" aria-hidden="true">{{ initials(name) }}</span>
</template>
