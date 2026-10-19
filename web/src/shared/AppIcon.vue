<script setup lang="ts">
import { computed, ref } from 'vue'

const props = defineProps<{ label: string; iconUrl?: string; size?: 'xs' | 'sm' | 'xl' }>()
const broken = ref(false)
const gradients: [string, string][] = [
  ['#ff8a3d', '#d9480f'], ['#8a63ff', '#1c7ed6'], ['#38c97f', '#137a47'], ['#4c6ef5', '#3b2fc9'],
  ['#ff6b9a', '#b81f56'], ['#3bc9db', '#0b7285'], ['#f59f00', '#e8590c'], ['#20c997', '#087f5b'],
  ['#9775fa', '#6741d9'], ['#4dabf7', '#1864ab'], ['#ff8787', '#c2255c'], ['#748ffc', '#4263eb'],
]
// An app without an icon shows its initials on a colour picked from its name.
const style = computed(() => {
  const [a, b] = gradients[Array.from(props.label).reduce((n, c) => n + c.charCodeAt(0), 0) % gradients.length] as [string, string]
  return { background: `linear-gradient(140deg, ${a}, ${b})` }
})
const letters = computed(() => props.label.trim().split(/\s+/).slice(0, 2).map((w) => w.charAt(0)).join('').toUpperCase())
</script>

<template>
  <span :class="['appicon', size]" :style="style" aria-hidden="true">
    <img v-if="iconUrl && !broken" :src="iconUrl" alt="" class="appicon-img" @error="broken = true" />
    <b v-else class="mono-l">{{ letters }}</b>
  </span>
</template>
