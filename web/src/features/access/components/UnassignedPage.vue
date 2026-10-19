<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import AppShell from '../../../shared/AppShell.vue'
import PersonAvatar from '../../../shared/PersonAvatar.vue'
import BackLink from '../../../shared/BackLink.vue'
import Icon from '../../../shared/Icon'
import RoleChip from '../../../shared/RoleChip.vue'
import { useDashboard } from '../../admin/composables/useDashboard'
import { ago } from '../../admin/format'
import type { AccessOverview, Group } from '../../../infrastructure/api/types.gen'
import { useAccess } from '../composables/useAccess'

const { dashboard, loadError } = useDashboard()
const { overview, loadError: accessError, addMember, removeMember, actionError } = useAccess()
const now = new Date()
const waiting = computed(() => dashboard.value?.unassigned ?? [])
// Every Group but the admins, which an Admin is made into on purpose, not from this queue.
const offered = (o: AccessOverview) => o.groups.filter((g) => !g.admins)
function toggle(group: Group, userId: string) {
  if (group.members.includes(userId)) removeMember(group.id, userId)
  else addMember(group.id, userId)
}
</script>

<template>
  <AppShell admin narrow>
    <BackLink to="/admin" label="Dashboard" />
    <header class="head"><h1>Unassigned</h1><div class="end"><span class="quiet">{{ waiting.length }} waiting</span></div></header>
    <p v-if="loadError || accessError || actionError" role="alert" class="alert"><Icon name="alert" />{{ loadError ?? accessError ?? actionError }}</p>
    <div v-if="waiting.length" class="un-list">
      <article v-for="u in waiting" :key="u.id" class="glass un-card">
        <div class="un-who"><PersonAvatar :name="u.name" size="lg" /><div><b>{{ u.name }}</b><small>{{ u.email }}</small></div><span class="soft mono">signed up {{ ago(u.confirmedAt, now) }}</span></div>
        <div class="un-groups" role="group" :aria-label="`Groups for ${u.name}`">
          <span class="label">Add to</span>
          <div v-if="overview" class="chips"><RoleChip v-for="g in offered(overview)" :key="g.id" :role="g.name" :held="g.members.includes(u.id)" @toggle="toggle(g, u.id)" /></div>
        </div>
        <div class="un-acts"><RouterLink class="btn ghost sm" :to="`/admin/people/${u.id}`">Open person</RouterLink></div>
      </article>
    </div>
    <div v-else-if="dashboard" class="glass empty"><span class="big-ico ok"><Icon name="check" /></span><p>No one waiting.</p></div>
  </AppShell>
</template>
