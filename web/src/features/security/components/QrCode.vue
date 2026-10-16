<script setup lang="ts">
import { computed } from 'vue'
import { encode } from 'uqr'

const props = defineProps<{ value: string; label: string }>()

// Each dark module is one unit square; the quiet zone around the code is four modules wide.
const qr = computed(() => encode(props.value, { border: 4 }))
const cells = computed(() => qr.value.data.flatMap((row, y) => row.flatMap((dark, x) => (dark ? [{ x, y }] : []))))
</script>

<template>
  <svg class="qr" :viewBox="`0 0 ${qr.size} ${qr.size}`" role="img" :aria-label="label" shape-rendering="crispEdges">
    <rect :width="qr.size" :height="qr.size" fill="#ffffff" />
    <rect v-for="c in cells" :key="`${c.x},${c.y}`" :x="c.x" :y="c.y" width="1" height="1" fill="#000000" />
  </svg>
</template>

<style scoped>
.qr {
  width: min(100%, 14rem);
  height: auto;
}
</style>
