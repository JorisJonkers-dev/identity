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
</script>

<template>
  <AdminLayout>
    <div class="head">
      <div>
        <h1>Estate sign-in</h1>
        <p class="sub">Last 24 hours.<template v-if="dashboard?.grafanaUrl"> Charts over time are in Grafana.</template></p>
      </div>
      <a v-if="dashboard?.grafanaUrl" class="ext" :href="dashboard.grafanaUrl" target="_blank" rel="noopener">Open in Grafana ↗</a>
    </div>
    <p v-if="isLoading" class="muted">Loading…</p>
    <p v-else-if="loadError" role="alert" class="error">{{ loadError }}</p>
    <template v-else-if="dashboard">
      <ul class="stats" aria-label="Counts">
        <li class="stat">
          <small>Users</small><b>{{ dashboard.users }}</b><span class="delta">+{{ dashboard.usersThisWeek }} this week</span>
        </li>
        <li class="stat" :class="{ warn: dashboard.unassigned.length > 0 }">
          <small>Unassigned</small><b>{{ dashboard.unassigned.length }}</b><span class="delta">waiting for a Group</span>
        </li>
        <li class="stat">
          <small>Sign-ins</small><b>{{ dashboard.signIns }}</b><span class="delta">{{ dashboard.failedSignIns }} failed</span>
        </li>
        <li class="stat">
          <small>Active sessions</small><b>{{ dashboard.activeSessions }}</b>
          <span class="delta">across {{ dashboard.sessionUsers }} {{ dashboard.sessionUsers === 1 ? 'User' : 'Users' }}</span>
        </li>
      </ul>
      <div class="two">
        <section class="panel" aria-labelledby="chart-title">
          <h2 id="chart-title">Sign-ins per hour <span>last 24h</span></h2>
          <SignInChart :hours="dashboard.hours" />
          <div class="keys" aria-hidden="true"><span><i></i>sign-ins</span><span><i class="f"></i>failed</span></div>
        </section>
        <section class="panel" aria-labelledby="queue-title">
          <h2 id="queue-title">Unassigned queue <span>new sign-ups</span></h2>
          <p v-if="dashboard.unassigned.length === 0" class="muted">No Users are waiting for a Group.</p>
          <ul v-else class="list" aria-labelledby="queue-title">
            <li v-for="u in dashboard.unassigned.slice(0, shown)" :key="u.id">
              <span class="av s" aria-hidden="true">{{ initials(u.name) }}</span>
              <div class="grow">
                <b>{{ u.name }}</b><small>{{ u.email }} · confirmed {{ ago(u.confirmedAt, now) }}</small>
              </div>
              <RouterLink class="btn" to="/access#waiting-title" :aria-label="`Add ${u.name} to a Group`">Add to a Group</RouterLink>
            </li>
          </ul>
          <RouterLink v-if="dashboard.unassigned.length > shown" class="ext more" to="/access#waiting-title">
            See all {{ dashboard.unassigned.length }} on the access page
          </RouterLink>
        </section>
      </div>
    </template>
  </AdminLayout>
</template>

<style scoped>
.stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 12px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.stat {
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: 12px;
  padding: 14px;
  display: grid;
  gap: 4px;
}
.stat small {
  color: var(--ink-3);
  font-size: 12px;
}
.stat b {
  font: 700 28px/1 var(--display);
  letter-spacing: -0.02em;
  font-variant-numeric: tabular-nums;
}
.stat .delta {
  font: 11.5px var(--mono);
  color: var(--ink-3);
}
.stat.warn {
  border-color: color-mix(in srgb, var(--warn) 40%, var(--line));
}
.stat.warn b {
  color: var(--warn);
}
.two {
  display: grid;
  grid-template-columns: minmax(0, 1.6fr) minmax(0, 1fr);
  gap: 12px;
}
.more {
  display: inline-block;
  margin-top: 10px;
}
.keys {
  display: flex;
  gap: 14px;
  font-size: 12px;
  color: var(--ink-3);
  margin-top: 6px;
  flex-wrap: wrap;
}
.keys span {
  display: inline-flex;
  gap: 6px;
  align-items: center;
}
.keys i {
  width: 14px;
  height: 2px;
  background: var(--accent);
}
.keys i.f {
  background: var(--bad);
}
@media (max-width: 900px) {
  .two {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
