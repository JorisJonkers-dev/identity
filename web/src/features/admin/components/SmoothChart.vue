<script setup lang="ts">
import { computed, useId } from 'vue'
import { smoothChart, ticks } from '../chart'

const props = defineProps<{ values: number[]; labels: string[]; tone: 'ok' | 'bad'; summary: string }>()
const id = useId()
const max = computed(() => ticks(Math.max(1, ...props.values))[3])
const chart = computed(() => smoothChart(props.values, props.labels, max.value))
const marks = computed(() => [0, 6, 12, 18, props.labels.length - 1].map((i) => ({ i, label: props.labels[i] as string })))
</script>

<template>
  <svg :class="['line-chart', tone === 'ok' ? 'okc-line' : 'bad-line']" viewBox="0 0 560 200" role="img" :aria-label="summary">
    <defs>
      <linearGradient :id="id" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0" stop-color="currentColor" stop-opacity=".22" />
        <stop offset="1" stop-color="currentColor" stop-opacity="0" />
      </linearGradient>
    </defs>
    <g v-for="g in chart.grid" :key="g.label">
      <line class="gl" x1="0" x2="534" :y1="g.y" :y2="g.y" />
      <text class="yl" x="560" :y="g.y + 4" text-anchor="end">{{ g.label }}</text>
    </g>
    <line class="base" x1="0" x2="534" y1="176" y2="176" />
    <path class="area" :fill="`url(#${id})`" :d="chart.area" />
    <path class="ln" :d="chart.line" />
    <circle class="peak" :cx="chart.peak.x" :cy="chart.peak.y" r="4.5" />
    <text v-for="(m, k) in marks" :key="m.i" class="xl" :x="chart.xs[m.i]" y="196" :text-anchor="k === 0 ? 'start' : k === marks.length - 1 ? 'end' : 'middle'">{{ m.label }}</text>
    <rect v-for="(h, k) in chart.hover" :key="k" class="hov" :x="h.x" y="10" :width="h.width" height="166"><title>{{ h.label }}</title></rect>
  </svg>
</template>
