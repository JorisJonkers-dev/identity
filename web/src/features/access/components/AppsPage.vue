<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import AppIcon from '../../../shared/AppIcon.vue'
import AppShell from '../../../shared/AppShell.vue'
import GlassSelect from '../../../shared/GlassSelect.vue'
import Icon from '../../../shared/Icon'
import { useAccess } from '../composables/useAccess'
import { allRoles } from '../model'

const router = useRouter()
const { overview, loadError } = useAccess()
const find = ref('')
const role = ref('')
const sort = ref<'name' | 'groups'>('name')
const groupsOn = (clientId: string) => new Set(overview.value?.assignments.filter((a) => a.clientId === clientId).map((a) => a.groupId)).size
const roleOptions = computed(() => [{ value: '', label: 'Any role' }, ...(overview.value ? allRoles(overview.value) : []).map((r) => ({ value: r, label: r }))])
const sortOptions = [
  { value: 'name' as const, label: 'A to Z' },
  { value: 'groups' as const, label: 'Most Groups' },
]
const apps = computed(() => {
  const q = find.value.trim().toLowerCase()
  return (overview.value?.clients ?? [])
    .filter((c) => (!q || c.label.toLowerCase().includes(q) || c.id.includes(q)) && (!role.value || c.roles.includes(role.value)))
    .sort((a, b) => (sort.value === 'groups' ? groupsOn(b.id) - groupsOn(a.id) : 0) || a.label.localeCompare(b.label))
})
</script>

<template>
  <AppShell admin>
    <header class="head"><h1>Apps</h1><div class="end"><span class="quiet">{{ overview?.clients.length ?? 0 }} apps</span></div></header>
    <p v-if="loadError" role="alert" class="alert"><Icon name="alert" />{{ loadError }}</p>
    <div class="tools bar-tools glass">
      <label class="search-sm"><Icon name="search" /><input v-model="find" type="search" placeholder="Find an app" aria-label="Find an app" /></label>
      <GlassSelect v-model="role" :options="roleOptions" label="Role" icon="key" />
      <GlassSelect v-model="sort" :options="sortOptions" label="Sort" icon="sort" />
    </div>
    <div class="glass lt">
      <div class="lt-h cols-apps2"><span>App</span><span>Id</span><span>Roles</span><span>Groups</span><span /></div>
      <button v-for="c in apps" :key="c.id" type="button" class="lt-r cols-apps2 row-btn" @click="router.push(`/admin/apps/${c.id}`)">
        <span class="app-c"><AppIcon :label="c.label" size="sm" />{{ c.label }}</span>
        <span class="muted">{{ c.id }}</span>
        <span class="roles"><span v-for="r in c.roles" :key="r" class="role">{{ r }}</span></span>
        <span class="muted">{{ groupsOn(c.id) }} {{ groupsOn(c.id) === 1 ? 'Group' : 'Groups' }}</span>
        <Icon name="chevr" class="chevr" />
      </button>
      <p v-if="overview && apps.length === 0" class="empty-state">No app matches.</p>
    </div>
  </AppShell>
</template>
