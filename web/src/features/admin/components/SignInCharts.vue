<script setup lang="ts">
import { computed } from 'vue'
import type { Dashboard, SignInHour } from '../../../infrastructure/api/types.gen'
import SmoothChart from './SmoothChart.vue'

/** The last day's sign-ins as two lines: those that got through, and those that failed. */
const props = defineProps<{ dashboard: Dashboard }>()
const at = (h: SignInHour) => new Date(h.at).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', hour12: false })
const s = computed(() => {
  const labels = props.dashboard.hours.map(at)
  const peak = (values: number[]) => {
    const i = values.reduce((best, v, k) => (v > (values[best] as number) ? k : best), 0)
    return values[i] ? `peak ${String(values[i])} at ${labels[i] as string}` : 'none'
  }
  const ok = props.dashboard.hours.map((h) => h.signIns - h.failed)
  const failed = props.dashboard.hours.map((h) => h.failed)
  return { labels, ok: { values: ok, peak: peak(ok) }, failed: { values: failed, peak: peak(failed) } }
})
</script>

<template>
  <div class="twin">
    <section class="glass panel chart-card ok-card">
      <div class="chart-h"><span class="chart-k"><i />Signed in</span><b>{{ dashboard.signIns - dashboard.failedSignIns }}</b><small>{{ s.ok.peak }}</small></div>
      <SmoothChart :values="s.ok.values" :labels="s.labels" tone="ok" :summary="`Successful sign-ins per hour over the last 24 hours, ${s.ok.peak}.`" />
    </section>
    <section class="glass panel chart-card bad-card">
      <div class="chart-h"><span class="chart-k"><i />Failed</span><b>{{ dashboard.failedSignIns }}</b><small>{{ s.failed.peak }}</small></div>
      <SmoothChart :values="s.failed.values" :labels="s.labels" tone="bad" :summary="`Failed sign-ins per hour over the last 24 hours, ${s.failed.peak}.`" />
    </section>
  </div>
</template>
