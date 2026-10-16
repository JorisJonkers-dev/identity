<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { useDashboard } from '../composables/useDashboard'
import { ago, initials } from '../format'
import AdminLayout from './AdminLayout.vue'
import SignInChart from './SignInChart.vue'

const { dashboard, isLoading, loadError } = useDashboard()
const now = new Date()
// The queue shows the newest few, so it never outgrows the chart beside it.
const shown = 5

const card = 'grid gap-1 rounded-xl border bg-surface p-3.5'
const stat = `${card} border-line`
// The Unassigned count draws the eye while anyone waits.
const attention = `${card} border-[color:color-mix(in_srgb,var(--color-warn)_40%,var(--color-line))]`
const label = 'text-xs text-ink-3'
const figure = 'font-display text-[28px]/none font-bold tracking-[-0.02em] tabular-nums'
const delta = 'font-mono text-[11.5px] text-ink-3'
const heading = 'mt-0 mb-2.5 flex items-center justify-between gap-2 text-[13px] font-semibold'
</script>

<template>
  <AdminLayout>
    <div class="flex flex-wrap items-end justify-between gap-3">
      <div>
        <h1 class="m-0 font-display text-[22px]/[1.15] font-bold tracking-[-0.02em] text-balance">Estate sign-in</h1>
        <p class="mt-0.5 mb-0 text-[13px] text-ink-3" data-test="subtitle">
          Last 24 hours.<template v-if="dashboard?.grafanaUrl"> Charts over time are in Grafana.</template>
        </p>
      </div>
      <a
        v-if="dashboard?.grafanaUrl"
        class="text-[12.5px] font-medium text-accent no-underline"
        :href="dashboard.grafanaUrl"
        target="_blank"
        rel="noopener"
      >Open in Grafana ↗</a>
    </div>
    <p v-if="isLoading" class="m-0 text-ink-3">Loading…</p>
    <p v-else-if="loadError" role="alert" class="m-0 text-bad">{{ loadError }}</p>
    <template v-else-if="dashboard">
      <ul class="m-0 grid list-none grid-cols-[repeat(auto-fit,minmax(140px,1fr))] gap-3 p-0" aria-label="Counts">
        <li :class="stat">
          <small :class="label">Users</small><b :class="figure">{{ dashboard.users }}</b>
          <span :class="delta">+{{ dashboard.usersThisWeek }} this week</span>
        </li>
        <li
          :class="dashboard.unassigned.length > 0 ? attention : stat"
          :data-attention="dashboard.unassigned.length > 0 || undefined"
        >
          <small :class="label">Unassigned</small>
          <b :class="[figure, dashboard.unassigned.length > 0 && 'text-warn']">{{ dashboard.unassigned.length }}</b>
          <span :class="delta">waiting for a Group</span>
        </li>
        <li :class="stat">
          <small :class="label">Sign-ins</small><b :class="figure">{{ dashboard.signIns }}</b>
          <span :class="delta">{{ dashboard.failedSignIns }} failed</span>
        </li>
        <li :class="stat">
          <small :class="label">Active sessions</small><b :class="figure">{{ dashboard.activeSessions }}</b>
          <span :class="delta">across {{ dashboard.sessionUsers }} {{ dashboard.sessionUsers === 1 ? 'User' : 'Users' }}</span>
        </li>
      </ul>
      <div class="grid grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)] gap-3 max-[900px]:grid-cols-1">
        <section class="panel" aria-labelledby="chart-title">
          <h2 id="chart-title" :class="heading">Sign-ins per hour <span class="font-normal text-ink-3">last 24h</span></h2>
          <SignInChart :hours="dashboard.hours" />
        </section>
        <section class="panel" aria-labelledby="queue-title">
          <h2 id="queue-title" :class="heading">Unassigned queue <span class="font-normal text-ink-3">new sign-ups</span></h2>
          <p v-if="dashboard.unassigned.length === 0" class="m-0 text-ink-3">No Users are waiting for a Group.</p>
          <ul v-else class="m-0 grid list-none p-0" aria-labelledby="queue-title">
            <li
              v-for="u in dashboard.unassigned.slice(0, shown)"
              :key="u.id"
              class="flex min-w-0 items-center gap-3 border-t border-line py-2.5 first:border-t-0 first:pt-0"
            >
              <span
                class="grid size-6 flex-none place-items-center rounded-full bg-accent-soft text-[10.5px] font-semibold text-accent"
                aria-hidden="true"
              >{{ initials(u.name) }}</span>
              <div class="min-w-0 flex-1">
                <b>{{ u.name }}</b>
                <small class="block truncate text-xs text-ink-3">{{ u.email }} · confirmed {{ ago(u.confirmedAt, now) }}</small>
              </div>
              <RouterLink class="btn" to="/access#waiting-title" :aria-label="`Add ${u.name} to a Group`">Add to a Group</RouterLink>
            </li>
          </ul>
          <RouterLink
            v-if="dashboard.unassigned.length > shown"
            class="mt-2.5 inline-block text-[12.5px] font-medium text-accent no-underline"
            to="/access#waiting-title"
          >
            See all {{ dashboard.unassigned.length }} on the access page
          </RouterLink>
        </section>
      </div>
    </template>
  </AdminLayout>
</template>
