<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import AppShell from '../../../shared/AppShell.vue'
import Avatar from '../../../shared/Avatar.vue'
import Icon from '../../../shared/Icon'
import { useDashboard } from '../composables/useDashboard'
import { ago, counted } from '../format'
import SmoothChart from './SmoothChart.vue'

const { dashboard, loadError } = useDashboard()
const now = new Date()
const hours = computed(() => dashboard.value?.hours ?? [])
const labels = computed(() => hours.value.map((h) => new Date(h.at).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', hour12: false })))
const ok = computed(() => hours.value.map((h) => h.signIns - h.failed))
const failed = computed(() => hours.value.map((h) => h.failed))
function peak(values: number[]): string {
  const at = values.reduce((best, v, i) => (v > (values[best] ?? 0) ? i : best), 0)
  return values[at] ? `peak ${String(values[at])} at ${labels.value[at] ?? ''}` : 'none'
}
</script>

<template>
  <AppShell admin>
    <header class="head">
      <h1>Dashboard</h1>
      <div class="end">
        <span class="quiet">Last 24 hours</span>
        <a v-if="dashboard?.grafanaUrl" class="btn ghost" :href="dashboard.grafanaUrl">Grafana<Icon name="out" /></a>
      </div>
    </header>
    <p v-if="loadError" role="alert" class="alert"><Icon name="alert" />{{ loadError }}</p>
    <template v-if="dashboard">
      <section class="glass stats">
        <RouterLink class="stat" to="/admin/people"><span class="lbl">People</span><span class="num">{{ dashboard.users }}</span><span class="meta"><span class="up">+{{ dashboard.usersThisWeek }}</span> this week</span></RouterLink>
        <RouterLink class="stat" to="/admin/unassigned">
          <span class="lbl"><i v-if="dashboard.unassigned.length" class="dot" />Unassigned</span><span class="num">{{ dashboard.unassigned.length }}</span>
          <span class="meta">{{ dashboard.unassigned[0] ? `newest ${ago(dashboard.unassigned[0].confirmedAt, now)}` : 'none waiting' }}</span>
        </RouterLink>
        <RouterLink class="stat" to="/admin/audit"><span class="lbl">Sign-ins</span><span class="num">{{ dashboard.signIns }}</span><span class="meta"><span class="bad">{{ dashboard.failedSignIns }}</span> failed</span></RouterLink>
        <RouterLink class="stat" to="/admin/people"><span class="lbl">Active sessions</span><span class="num">{{ dashboard.activeSessions }}</span><span class="meta">{{ counted(dashboard.sessionUsers, 'person').replace('persons', 'people') }}</span></RouterLink>
        <RouterLink class="stat" to="/admin/people"><span class="lbl"><i v-if="dashboard.tokensExpiring" class="dot" />Tokens expiring</span><span class="num">{{ dashboard.tokensExpiring }}</span><span class="meta">this week</span></RouterLink>
      </section>
      <div class="twin">
        <section class="glass panel chart-card ok-card">
          <div class="chart-h"><span class="chart-k"><i />Signed in</span><b>{{ dashboard.signIns - dashboard.failedSignIns }}</b><small>{{ peak(ok) }}</small></div>
          <SmoothChart :values="ok" :labels="labels" tone="ok" :summary="`Successful sign-ins per hour over the last 24 hours, ${peak(ok)}.`" />
        </section>
        <section class="glass panel chart-card bad-card">
          <div class="chart-h"><span class="chart-k"><i />Failed</span><b>{{ dashboard.failedSignIns }}</b><small>{{ peak(failed) }}</small></div>
          <SmoothChart :values="failed" :labels="labels" tone="bad" :summary="`Failed sign-ins per hour over the last 24 hours, ${peak(failed)}.`" />
        </section>
      </div>
      <section class="glass panel un-panel">
        <div class="panel-h"><h2>Unassigned<span v-if="dashboard.unassigned.length" class="count">{{ dashboard.unassigned.length }}</span></h2><RouterLink class="btn sm" to="/admin/unassigned">Assign<Icon name="arrow" /></RouterLink></div>
        <div v-if="dashboard.unassigned.length" class="queue two-col">
          <RouterLink v-for="u in dashboard.unassigned" :key="u.id" class="q" :to="`/admin/people/${u.id}`">
            <Avatar :name="u.name" /><div><b>{{ u.name }}</b><small>{{ u.email }} · {{ ago(u.confirmedAt, now) }}</small></div><Icon name="chevr" class="chevr" />
          </RouterLink>
        </div>
        <p v-else class="quiet-line"><Icon name="check" />No one waiting.</p>
      </section>
    </template>
  </AppShell>
</template>
