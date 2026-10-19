<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import AppIcon from '../../../shared/AppIcon.vue'
import AppShell from '../../../shared/AppShell.vue'
import Avatar from '../../../shared/Avatar.vue'
import GlassSelect from '../../../shared/GlassSelect.vue'
import Icon from '../../../shared/Icon'
import RoleChip from '../../../shared/RoleChip.vue'
import { useAccess } from '../composables/useAccess'
import { allRoles, appsOf, groupsOf, holds, membersOf } from '../model'

const route = useRoute()
const router = useRouter()
const { overview, loadError, actionError, createGroup, renameGroup, deleteGroup, addMember, removeMember, setRole } = useAccess()

const groups = computed(() => overview.value?.groups ?? [])
const selected = computed(() => {
  const id = typeof route.params.groupId === 'string' ? route.params.groupId : ''
  return groups.value.find((g) => g.id === id) ?? groups.value.find((g) => !g.admins) ?? groups.value[0]
})
const tab = computed(() => (route.query.tab === 'members' ? 'members' : 'access'))
function go(groupId: string, t = tab.value) {
  void router.replace({ path: `/admin/groups/${groupId}`, query: t === 'members' ? { tab: 'members' } : {} })
}

// Access: every app, filtered by text, role and whether the Group holds any role on it.
const find = ref('')
const role = ref('')
const onlyHeld = ref(false)
const sort = ref<'held' | 'name'>('held')
const roleOptions = computed(() => [{ value: '', label: 'Any role' }, ...(overview.value ? allRoles(overview.value) : []).map((r) => ({ value: r, label: r }))])
const sortOptions = [
  { value: 'held' as const, label: 'With access first' },
  { value: 'name' as const, label: 'A to Z' },
]
const held = (clientId: string) => (overview.value && selected.value ? appsOf(overview.value, selected.value.id).includes(clientId) : false)
const apps = computed(() => {
  const o = overview.value
  if (!o) return []
  const q = find.value.trim().toLowerCase()
  return o.clients
    .filter((c) => (!q || c.label.toLowerCase().includes(q) || c.id.includes(q)) && (!role.value || c.roles.includes(role.value)) && (!onlyHeld.value || held(c.id)))
    .sort((a, b) => (sort.value === 'held' ? Number(held(b.id)) - Number(held(a.id)) : 0) || a.label.localeCompare(b.label))
})
const heldCount = computed(() => (overview.value && selected.value ? appsOf(overview.value, selected.value.id).length : 0))

// The last role change, so the toast can say it and undo it.
const last = ref<{ clientId: string; label: string; role: string; held: boolean } | null>(null)
let timer: ReturnType<typeof setTimeout> | undefined
function toggle(clientId: string, label: string, r: string) {
  if (!overview.value || !selected.value) return
  const now = !holds(overview.value, selected.value.id, clientId, r)
  setRole(selected.value.id, clientId, r, now)
  last.value = { clientId, label, role: r, held: now }
  clearTimeout(timer)
  timer = setTimeout(() => (last.value = null), 4000)
}
function undo() {
  if (last.value && selected.value) setRole(selected.value.id, last.value.clientId, last.value.role, !last.value.held)
  last.value = null
}

// Members: the people in the Group, and everyone else to add.
const members = computed(() => (overview.value && selected.value ? membersOf(overview.value, selected.value) : []))
const adding = ref('')
const addOpen = ref(false)
const others = computed(() => {
  const o = overview.value
  if (!o || !selected.value) return []
  const q = adding.value.trim().toLowerCase()
  return o.users
    .filter((u) => !selected.value!.members.includes(u.id) && (!q || u.name.toLowerCase().includes(q) || u.email.includes(q)))
    .map((u) => ({ ...u, unassigned: groupsOf(o, u.id).length === 0 }))
    .sort((a, b) => Number(b.unassigned) - Number(a.unassigned) || a.name.localeCompare(b.name))
})
const alsoIn = (userId: string) =>
  overview.value ? groupsOf(overview.value, userId).filter((g) => g.id !== selected.value?.id).map((g) => g.name).join(' · ') : ''

// Dialogs: a new Group, a new name, deleting one.
const dialog = ref<'new' | 'rename' | 'delete' | null>(null)
const draft = ref('')
function open(kind: 'new' | 'rename' | 'delete') {
  draft.value = kind === 'rename' ? (selected.value?.name ?? '') : ''
  dialog.value = kind
}
function submit() {
  if (dialog.value === 'new') createGroup(draft.value, () => undefined)
  else if (dialog.value === 'rename' && selected.value) renameGroup(selected.value.id, draft.value)
  else if (dialog.value === 'delete' && selected.value) {
    deleteGroup(selected.value.id)
    void router.replace('/admin/groups')
  }
  dialog.value = null
}
watch(selected, () => {
  find.value = ''
  last.value = null
})
</script>

<template>
  <AppShell admin>
    <header class="head">
      <h1>Groups</h1>
      <div class="end"><button class="btn primary" type="button" @click="open('new')"><Icon name="plus" />New Group</button></div>
    </header>
    <p v-if="loadError || actionError" role="alert" class="alert"><Icon name="alert" />{{ loadError ?? actionError }}</p>
    <div v-if="overview && selected" class="glass g-split">
      <nav class="g-list" aria-label="Groups">
        <p class="g-list-h">Groups <span>{{ groups.length }}</span></p>
        <RouterLink v-for="g in groups" :key="g.id" class="g-item" :to="`/admin/groups/${g.id}`" :aria-current="g.id === selected.id ? 'page' : undefined">
          <b><Icon v-if="g.admins" name="shield" />{{ g.name }}</b>
          <small>{{ g.members.length }} people · {{ g.admins ? 'admin console' : `${appsOf(overview, g.id).length} apps` }}</small>
        </RouterLink>
      </nav>
      <section class="g-detail" :aria-label="selected.name">
        <div class="g-head">
          <div>
            <h2><Icon v-if="selected.admins" name="shield" />{{ selected.name }}</h2>
            <span class="members"><span class="stack"><Avatar v-for="m in members.slice(0, 4)" :key="m.id" :name="m.name" size="sm" /></span>{{ members.length }} people</span>
          </div>
          <div class="end">
            <button v-if="!selected.admins" class="btn ghost sm" type="button" @click="open('rename')"><Icon name="pencil" />Rename</button>
            <button v-if="!selected.admins" class="btn ghost sm danger-t" type="button" @click="open('delete')"><Icon name="trash" />Delete</button>
          </div>
        </div>
        <nav class="g-tabs" :aria-label="selected.name">
          <button type="button" class="g-tab" :aria-current="tab === 'access' ? 'page' : undefined" @click="go(selected.id, 'access')">Access <span class="n">{{ selected.admins ? 'admin' : heldCount }}</span></button>
          <button type="button" class="g-tab" :aria-current="tab === 'members' ? 'page' : undefined" @click="go(selected.id, 'members')">Members <span class="n">{{ members.length }}</span></button>
        </nav>

        <div v-if="tab === 'access'" class="g-body">
          <p v-if="selected.admins" class="admin-note"><Icon name="shield" />Admins manage identity: Groups, apps, people and the audit log. No app roles.</p>
          <template v-else>
            <div class="tools">
              <label class="search-sm"><Icon name="search" /><input v-model="find" type="search" placeholder="Find an app" aria-label="Find an app" /></label>
              <GlassSelect v-model="role" :options="roleOptions" label="Role" icon="key" />
              <GlassSelect v-model="sort" :options="sortOptions" label="Sort" icon="sort" />
              <div class="seg" role="group" aria-label="Show">
                <button type="button" :aria-pressed="!onlyHeld" @click="onlyHeld = false">All {{ overview.clients.length }}</button>
                <button type="button" :aria-pressed="onlyHeld" @click="onlyHeld = true">With access {{ heldCount }}</button>
              </div>
            </div>
            <div class="acc">
              <div v-for="c in apps" :key="c.id" :class="['acc-row', { held: held(c.id) }]">
                <AppIcon :label="c.label" size="sm" />
                <span class="acc-n"><b>{{ c.label }}</b><small>{{ c.id }}</small></span>
                <div class="chips" role="group" :aria-label="`${selected.name} on ${c.label}`">
                  <RoleChip v-for="r in c.roles" :key="r" :role="r" :held="holds(overview, selected.id, c.id, r)" @toggle="toggle(c.id, c.label, r)" />
                </div>
              </div>
              <p v-if="apps.length === 0" class="empty-state">No app matches.</p>
            </div>
          </template>
        </div>

        <div v-else class="g-body">
          <div class="tools">
            <div class="combo">
              <label class="search-sm wide"><Icon name="userplus" /><input v-model="adding" type="search" placeholder="Add people" aria-label="Add people" @focus="addOpen = true" @keydown.esc="addOpen = false" /></label>
              <div v-if="addOpen" class="sugg-list glass strong">
                <p class="sugg-h">Click to add · Esc to close</p>
                <button v-for="u in others" :key="u.id" class="sugg" type="button" @click="addMember(selected.id, u.id)">
                  <Avatar :name="u.name" size="sm" /><span class="grow"><b>{{ u.name }}</b><small>{{ u.email }}</small></span>
                  <span v-if="u.unassigned" class="gtag warn">Unassigned</span><span class="add-k"><Icon name="plus" />Add</span>
                </button>
                <p v-if="others.length === 0" class="sugg-h">Everyone is in {{ selected.name }}.</p>
              </div>
            </div>
            <button v-if="addOpen" class="btn ghost sm" type="button" @click="addOpen = false">Done</button>
          </div>
          <div class="m-list">
            <div v-for="m in members" :key="m.id" class="m-row">
              <Avatar :name="m.name" />
              <RouterLink class="grow m-link" :to="`/admin/people/${m.id}`"><b>{{ m.name }}</b><small>{{ m.email }}</small></RouterLink>
              <span class="also">{{ alsoIn(m.id) }}</span>
              <button class="icon-btn sm" type="button" :aria-label="`Remove ${m.name} from ${selected.name}`" @click="removeMember(selected.id, m.id)"><Icon name="x" /></button>
            </div>
            <p v-if="members.length === 0" class="empty-state">No one yet.</p>
          </div>
        </div>
      </section>
    </div>

    <div v-if="last" class="toast glass strong" role="status">
      <Icon name="check" class="ok" />
      <span><b>{{ selected?.name }}</b> {{ last.held ? 'now holds' : 'no longer holds' }} <b>{{ last.role }}</b> on {{ last.label }}</span>
      <button class="btn ghost sm" type="button" @click="undo">Undo</button>
    </div>

    <div v-if="dialog" class="scrim" @click.self="dialog = null">
      <form class="dialog glass strong" role="dialog" aria-modal="true" aria-labelledby="g-dlg" @submit.prevent="submit">
        <template v-if="dialog === 'delete'">
          <h2 id="g-dlg">Delete {{ selected?.name }}?</h2>
          <p>{{ members.length }} people lose what it gives them.</p>
          <div class="btn-row end"><button class="btn" type="button" @click="dialog = null">Cancel</button><button class="btn danger-solid" type="submit">Delete Group</button></div>
        </template>
        <template v-else>
          <h2 id="g-dlg">{{ dialog === 'new' ? 'New Group' : `Rename ${selected?.name}` }}</h2>
          <div class="field"><label class="label" for="g-name">Name</label><input id="g-name" v-model="draft" class="input md" required maxlength="64" /></div>
          <div class="btn-row end"><button class="btn" type="button" @click="dialog = null">Cancel</button><button class="btn primary" type="submit">{{ dialog === 'new' ? 'Create Group' : 'Save name' }}</button></div>
        </template>
      </form>
    </div>
  </AppShell>
</template>
