<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import AppShell from '../../../shared/AppShell.vue'
import Avatar from '../../../shared/Avatar.vue'
import Crumb from '../../../shared/Crumb.vue'
import Icon from '../../../shared/Icon'
import RoleChip from '../../../shared/RoleChip.vue'
import { useDashboard } from '../../admin/composables/useDashboard'
import { ago } from '../../admin/format'
import { useAccess } from '../composables/useAccess'

const { dashboard, loadError } = useDashboard()
const { overview, addMember, removeMember, actionError } = useAccess()
const now = new Date()
const waiting = computed(() => dashboard.value?.unassigned ?? [])
const groups = computed(() => overview.value?.groups.filter((g) => !g.admins) ?? [])
const inGroup = (groupId: string, userId: string) => overview.value?.groups.find((g) => g.id === groupId)?.members.includes(userId) === true
function toggle(groupId: string, userId: string) {
  if (inGroup(groupId, userId)) removeMember(groupId, userId)
  else addMember(groupId, userId)
}
</script>

<template>
  <AppShell admin narrow>
    <Crumb to="/admin" label="Dashboard" />
    <header class="head"><h1>Unassigned</h1><div class="end"><span class="quiet">{{ waiting.length }} waiting</span></div></header>
    <p v-if="loadError || actionError" role="alert" class="alert"><Icon name="alert" />{{ loadError ?? actionError }}</p>
    <div v-if="waiting.length" class="un-list">
      <article v-for="u in waiting" :key="u.id" class="glass un-card">
        <div class="un-who"><Avatar :name="u.name" size="lg" /><div><b>{{ u.name }}</b><small>{{ u.email }}</small></div><span class="soft mono">signed up {{ ago(u.confirmedAt, now) }}</span></div>
        <div class="un-groups" role="group" :aria-label="`Groups for ${u.name}`">
          <span class="label">Add to</span>
          <div class="chips"><RoleChip v-for="g in groups" :key="g.id" :role="g.name" :held="inGroup(g.id, u.id)" @toggle="toggle(g.id, u.id)" /></div>
        </div>
        <div class="un-acts"><RouterLink class="btn ghost sm" :to="`/admin/people/${u.id}`">Open person</RouterLink></div>
      </article>
    </div>
    <div v-else-if="dashboard" class="glass empty"><span class="big-ico ok"><Icon name="check" /></span><p>No one waiting.</p></div>
  </AppShell>
</template>
