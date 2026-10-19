<script setup lang="ts">
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'
import AppShell from '../../../shared/AppShell.vue'
import PersonAvatar from '../../../shared/PersonAvatar.vue'
import GlassSelect from '../../../shared/GlassSelect.vue'
import Icon from '../../../shared/Icon'
import { useAccess } from '../composables/useAccess'
import { groupsOf, unassigned } from '../model'

const { overview, loadError, actionError, addMember } = useAccess()
const find = ref('')
const group = ref('')
const sort = ref<'name' | 'unassigned'>('unassigned')
const picked = ref(new Set<string>())
const groupOptions = computed(() => [
  { value: '', label: 'Any Group' },
  { value: 'none', label: 'Unassigned' },
  { value: 'direct', label: 'With a direct role' },
  ...(overview.value?.groups ?? []).map((g) => ({ value: g.id, label: g.name })),
])
const addOptions = computed(() => (overview.value?.groups ?? []).filter((g) => !g.admins).map((g) => ({ value: g.id, label: g.name })))
const sortOptions = [
  { value: 'unassigned' as const, label: 'Unassigned first' },
  { value: 'name' as const, label: 'A to Z' },
]
const people = computed(() => {
  const o = overview.value
  if (!o) return []
  const q = find.value.trim().toLowerCase()
  return o.users
    .map((u) => ({ ...u, groups: groupsOf(o, u.id), waiting: unassigned(o, u.id) }))
    .filter((u) => (!q || u.name.toLowerCase().includes(q) || u.email.includes(q)) && (!group.value || (group.value === 'none' ? u.waiting : group.value === 'direct' ? o.userAssignments.some((a) => a.userId === u.id) : u.groups.some((g) => g.id === group.value))))
    .sort((a, b) => (sort.value === 'unassigned' ? Number(b.waiting) - Number(a.waiting) : 0) || a.name.localeCompare(b.name))
})
function pick(id: string, on: boolean) {
  const next = new Set(picked.value)
  if (on) next.add(id)
  else next.delete(id)
  picked.value = next
}
function bulkAdd(groupId: string) {
  for (const id of picked.value) addMember(groupId, id)
  picked.value = new Set()
}
</script>

<template>
  <AppShell admin>
    <header class="head"><h1>People</h1><div class="end"><span class="quiet">{{ overview?.users.length ?? 0 }} people</span></div></header>
    <p v-if="loadError || actionError" role="alert" class="alert"><Icon name="alert" />{{ loadError ?? actionError }}</p>
    <div class="tools bar-tools glass">
      <label class="search-sm"><Icon name="search" /><input v-model="find" type="search" placeholder="Find a person" aria-label="Find a person" /></label>
      <GlassSelect v-model="group" :options="groupOptions" label="Group" icon="users" />
      <GlassSelect v-model="sort" :options="sortOptions" label="Sort" icon="sort" />
    </div>
    <div class="glass lt">
      <div class="lt-h cols-people2"><span /><span>Person</span><span>Groups</span><span /></div>
      <div v-for="p in people" :key="p.id" :class="['lt-r cols-people2', { picked: picked.has(p.id) }]">
        <label class="pick"><input type="checkbox" :checked="picked.has(p.id)" :aria-label="`Select ${p.name}`" @change="pick(p.id, ($event.target as HTMLInputElement).checked)" /><span class="box"><Icon name="check" /></span></label>
        <RouterLink class="who" :to="`/admin/people/${p.id}`"><PersonAvatar :name="p.name" /><span><b>{{ p.name }}</b><small>{{ p.email }}</small></span></RouterLink>
        <span class="tags">
          <span v-for="g in p.groups" :key="g.id" class="gtag">{{ g.name }}</span>
          <span v-if="p.waiting" class="gtag warn">Unassigned</span>
        </span>
        <RouterLink class="row-go" :to="`/admin/people/${p.id}`" :aria-label="`Open ${p.name}`"><Icon name="chevr" class="chevr" /></RouterLink>
      </div>
      <p v-if="overview && people.length === 0" class="empty-state">No one matches.</p>
    </div>
    <div :class="['bulk glass strong', { shown: picked.size > 0 }]" role="region" aria-label="Selected people">
      <b>{{ picked.size }} selected</b>
      <GlassSelect :options="addOptions" label="Add selected to a Group" placeholder="Add to a Group" icon="userplus" small @pick="bulkAdd" />
      <button class="btn sm ghost" type="button" @click="picked = new Set()">Clear</button>
    </div>
  </AppShell>
</template>
