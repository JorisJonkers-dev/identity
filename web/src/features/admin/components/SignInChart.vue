<script setup lang="ts">
import { computed } from 'vue'
import type { SignInHour } from '../../../infrastructure/api/types.gen'
import { drawChart } from '../format'

const props = defineProps<{ hours: SignInHour[] }>()
const chart = computed(() => drawChart(props.hours))
</script>

<template>
  <svg class="spark" viewBox="0 0 480 150" role="img" :aria-label="chart.summary">
    <line class="grid" x1="30" y1="20" x2="470" y2="20" />
    <line class="grid" x1="30" y1="65" x2="470" y2="65" />
    <line class="grid" x1="30" y1="110" x2="470" y2="110" />
    <text x="24" y="24" text-anchor="end">{{ chart.max }}</text>
    <text x="24" y="69" text-anchor="end">{{ chart.max / 2 }}</text>
    <text x="24" y="114" text-anchor="end">0</text>
    <text x="30" y="132">{{ chart.first }}</text>
    <text :x="chart.middleX" y="132" text-anchor="middle">{{ chart.middle }}</text>
    <text x="470" y="132" text-anchor="end">{{ chart.last }}</text>
    <path class="area" :d="chart.area" />
    <polyline class="line" :points="chart.line" />
    <polyline class="fail" :points="chart.failed" />
    <circle class="end" r="3.5" :cx="chart.end.x" :cy="chart.end.y" />
  </svg>
</template>

<style scoped>
.spark {
  width: 100%;
  height: auto;
  display: block;
}
.spark text {
  fill: var(--ink-3);
  font: 10px var(--mono);
}
.spark .grid {
  stroke: var(--line);
  stroke-width: 1;
}
.spark .area {
  fill: var(--accent-soft);
}
.spark .line {
  fill: none;
  stroke: var(--accent);
  stroke-width: 2;
  stroke-linejoin: round;
}
.spark .fail {
  fill: none;
  stroke: var(--bad);
  stroke-width: 1.5;
  stroke-dasharray: 3 3;
}
.spark .end {
  fill: var(--accent);
}
</style>
