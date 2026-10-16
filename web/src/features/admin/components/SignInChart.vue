<script setup lang="ts">
import { computed } from 'vue'
import type { SignInHour } from '../../../infrastructure/api/types.gen'
import { drawChart } from '../format'

const props = defineProps<{ hours: SignInHour[] }>()
const chart = computed(() => drawChart(props.hours))
</script>

<template>
  <svg class="block h-auto w-full fill-ink-3 font-mono text-[10px]" viewBox="0 0 480 150" role="img" :aria-label="chart.summary">
    <g class="stroke-line">
      <line x1="30" y1="20" x2="470" y2="20" />
      <line x1="30" y1="65" x2="470" y2="65" />
      <line x1="30" y1="110" x2="470" y2="110" />
    </g>
    <text x="24" y="24" text-anchor="end">{{ chart.max }}</text>
    <text x="24" y="69" text-anchor="end">{{ chart.max / 2 }}</text>
    <text x="24" y="114" text-anchor="end">0</text>
    <text x="30" y="132">{{ chart.first }}</text>
    <text :x="chart.middleX" y="132" text-anchor="middle">{{ chart.middle }}</text>
    <text x="470" y="132" text-anchor="end">{{ chart.last }}</text>
    <path class="fill-accent-soft" :d="chart.area" />
    <polyline data-test="sign-ins" class="fill-none stroke-accent stroke-2 [stroke-linejoin:round]" :points="chart.line" />
    <polyline class="fill-none stroke-bad stroke-[1.5] [stroke-dasharray:3_3]" :points="chart.failed" />
    <circle class="fill-accent" r="3.5" :cx="chart.end.x" :cy="chart.end.y" />
  </svg>
  <div class="mt-1.5 flex flex-wrap gap-3.5 text-xs text-ink-3" aria-hidden="true">
    <span class="inline-flex items-center gap-1.5"><i class="h-0.5 w-3.5 bg-accent"></i>sign-ins</span>
    <span class="inline-flex items-center gap-1.5"><i class="h-0.5 w-3.5 bg-bad"></i>failed</span>
  </div>
</template>
