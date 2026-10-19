<script setup lang="ts">
import { RouterLink } from 'vue-router'
import AppShell from '../../../shared/AppShell.vue'
import PersonAvatar from '../../../shared/PersonAvatar.vue'
import Icon from '../../../shared/Icon'
import { useDashboard } from '../composables/useDashboard'
import { ago, counted } from '../format'
import SignInCharts from './SignInCharts.vue'

const { dashboard, loadError } = useDashboard()
const now = new Date()
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
        <RouterLink class="stat" to="/admin/people"><span class="lbl">Active sessions</span><span class="num">{{ dashboard.activeSessions }}</span><span class="meta">{{ counted(dashboard.sessionUsers, 'person', 'people') }}</span></RouterLink>
        <RouterLink class="stat" to="/admin/people"><span class="lbl"><i v-if="dashboard.tokensExpiring" class="dot" />Tokens expiring</span><span class="num">{{ dashboard.tokensExpiring }}</span><span class="meta">this week</span></RouterLink>
      </section>
      <SignInCharts :dashboard="dashboard" />
      <section class="glass panel un-panel">
        <div class="panel-h"><h2>Unassigned<span v-if="dashboard.unassigned.length" class="count">{{ dashboard.unassigned.length }}</span></h2><RouterLink class="btn sm" to="/admin/unassigned">Assign<Icon name="arrow" /></RouterLink></div>
        <div v-if="dashboard.unassigned.length" class="queue two-col">
          <RouterLink v-for="u in dashboard.unassigned.slice(0, 6)" :key="u.id" class="q" :to="`/admin/people/${u.id}`">
            <PersonAvatar :name="u.name" /><div><b>{{ u.name }}</b><small>{{ u.email }} · {{ ago(u.confirmedAt, now) }}</small></div><Icon name="chevr" class="chevr" />
          </RouterLink>
        </div>
        <p v-else class="quiet-line"><Icon name="check" />No one waiting.</p>
      </section>
    </template>
  </AppShell>
</template>
