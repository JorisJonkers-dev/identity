<script setup lang="ts">
import { computed, ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import type { AccessOverview, Group } from '../../../infrastructure/api/types.gen'
import AppIcon from '../../../shared/AppIcon.vue'
import DialogScrim from '../../../shared/DialogScrim.vue'
import GlassSelect from '../../../shared/GlassSelect.vue'
import Icon from '../../../shared/Icon'
import PersonAvatar from '../../../shared/PersonAvatar.vue'
import RoleChip from '../../../shared/RoleChip.vue'
import { useAccess } from '../composables/useAccess'
import { allRoles, appsOf, groupsOf, holds, membersOf } from '../model'

/** One Group: the apps it gives roles on, and the people in it. */
const props = defineProps<{ o: AccessOverview; group: Group }>()
const route = useRoute()
const router = useRouter()
const { actionError, renameGroup, deleteGroup, addMember, removeMember, setRole } = useAccess()

const tab = computed(() => (route.query.tab === 'members' ? 'members' : 'access'))
function go(members: boolean) {
  void router.replace({ path: `/admin/groups/${props.group.id}`, query: members ? { tab: 'members' } : {} })
}

// Access: every app, filtered by text, role and whether the Group holds any role on it.
const find = ref('')
const role = ref('')
const onlyHeld = ref(false)
const sort = ref<'held' | 'name'>('held')
const roleOptions = computed(() => [{ value: '', label: 'Any role' }, ...allRoles(props.o).map((r) => ({ value: r, label: r }))])
const sortOptions = [
  { value: 'held' as const, label: 'With access first' },
  { value: 'name' as const, label: 'A to Z' },
]
const heldApps = computed(() => appsOf(props.o, props.group.id))
const held = (clientId: string) => heldApps.value.includes(clientId)
const apps = computed(() => {
  const q = find.value.trim().toLowerCase()
  return props.o.clients
    .filter((c) => (!q || c.label.toLowerCase().includes(q) || c.id.includes(q)) && (!role.value || c.roles.includes(role.value)) && (!onlyHeld.value || held(c.id)))
    .sort((a, b) => (sort.value === 'held' ? Number(held(b.id)) - Number(held(a.id)) : 0) || a.label.localeCompare(b.label))
})

// The last role change, so the toast can say it and undo it.
interface Change {
  clientId: string
  label: string
  role: string
  held: boolean
}
const last = ref<Change | null>(null)
let timer: ReturnType<typeof setTimeout> | undefined
function toggle(clientId: string, label: string, r: string) {
  const now = !holds(props.o, props.group.id, clientId, r)
  setRole(props.group.id, clientId, r, now)
  last.value = { clientId, label, role: r, held: now }
  clearTimeout(timer)
  timer = setTimeout(() => (last.value = null), 4000)
}
function undo(change: Change) {
  setRole(props.group.id, change.clientId, change.role, !change.held)
  last.value = null
}

// Members: the people in the Group, and everyone else to add, those in no Group first.
const members = computed(() => membersOf(props.o, props.group))
const adding = ref('')
const addOpen = ref(false)
const others = computed(() => {
  const q = adding.value.trim().toLowerCase()
  return props.o.users
    .filter((u) => !props.group.members.includes(u.id) && (!q || u.name.toLowerCase().includes(q) || u.email.includes(q)))
    .map((u) => ({ ...u, unassigned: groupsOf(props.o, u.id).length === 0 }))
    .sort((a, b) => Number(b.unassigned) - Number(a.unassigned) || a.name.localeCompare(b.name))
})
const alsoIn = (userId: string) =>
  groupsOf(props.o, userId)
    .filter((g) => g.id !== props.group.id)
    .map((g) => g.name)
    .join(' · ')

// Dialogs: a new name, or deleting the Group.
const dialog = ref<'rename' | 'delete' | null>(null)
const draft = ref('')
function rename() {
  renameGroup(props.group.id, draft.value)
  dialog.value = null
}
function remove() {
  dialog.value = null
  deleteGroup(props.group.id, () => void router.replace('/admin/groups'))
}
</script>

<template>
  <section class="g-detail" :aria-label="group.name">
    <div class="g-head">
      <div>
        <h2><Icon v-if="group.admins" name="shield" />{{ group.name }}</h2>
        <span class="members"><span class="stack"><PersonAvatar v-for="m in members.slice(0, 4)" :key="m.id" :name="m.name" size="sm" /></span>{{ members.length }} people</span>
      </div>
      <div v-if="!group.admins" class="end">
        <button class="btn ghost sm" type="button" @click="draft = group.name; dialog = 'rename'"><Icon name="pencil" />Rename</button>
        <button class="btn ghost sm danger-t" type="button" @click="dialog = 'delete'"><Icon name="trash" />Delete</button>
      </div>
    </div>
    <p v-if="actionError" role="alert" class="alert"><Icon name="alert" />{{ actionError }}</p>
    <nav class="g-tabs" :aria-label="group.name">
      <button type="button" class="g-tab" :aria-current="tab === 'access' ? 'page' : undefined" @click="go(false)">Access <span class="n">{{ group.admins ? 'Admin' : heldApps.length }}</span></button>
      <button type="button" class="g-tab" :aria-current="tab === 'members' ? 'page' : undefined" @click="go(true)">Members <span class="n">{{ members.length }}</span></button>
    </nav>

    <div v-if="tab === 'access'" class="g-body">
      <p v-if="group.admins" class="admin-note"><Icon name="shield" />Admins manage identity: Groups, apps, people and the audit log. No app roles.</p>
      <template v-else>
        <div class="tools">
          <label class="search-sm"><Icon name="search" /><input v-model="find" type="search" placeholder="Find an app" aria-label="Find an app" /></label>
          <GlassSelect v-model="role" :options="roleOptions" label="Role" icon="key" />
          <GlassSelect v-model="sort" :options="sortOptions" label="Sort" icon="sort" />
          <div class="seg" role="group" aria-label="Show">
            <button type="button" :aria-pressed="!onlyHeld" @click="onlyHeld = false">All {{ o.clients.length }}</button>
            <button type="button" :aria-pressed="onlyHeld" @click="onlyHeld = true">With access {{ heldApps.length }}</button>
          </div>
        </div>
        <div class="acc">
          <div v-for="c in apps" :key="c.id" :class="['acc-row', { held: held(c.id) }]">
            <AppIcon :label="c.label" size="sm" />
            <span class="acc-n"><b>{{ c.label }}</b><small>{{ c.id }}</small></span>
            <div class="chips" role="group" :aria-label="`${group.name} on ${c.label}`">
              <RoleChip v-for="r in c.roles" :key="r" :role="r" :held="holds(o, group.id, c.id, r)" @toggle="toggle(c.id, c.label, r)" />
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
            <button v-for="u in others" :key="u.id" class="sugg" type="button" @click="addMember(group.id, u.id)">
              <PersonAvatar :name="u.name" size="sm" /><span class="grow"><b>{{ u.name }}</b><small>{{ u.email }}</small></span>
              <span v-if="u.unassigned" class="gtag warn">Unassigned</span><span class="add-k"><Icon name="plus" />Add</span>
            </button>
            <p v-if="others.length === 0" class="sugg-h">Everyone is in {{ group.name }}.</p>
          </div>
        </div>
        <button v-if="addOpen" class="btn ghost sm" type="button" @click="addOpen = false">Done</button>
      </div>
      <div class="m-list">
        <div v-for="m in members" :key="m.id" class="m-row">
          <PersonAvatar :name="m.name" />
          <RouterLink class="grow m-link" :to="`/admin/people/${m.id}`"><b>{{ m.name }}</b><small>{{ m.email }}</small></RouterLink>
          <span class="also">{{ alsoIn(m.id) }}</span>
          <button class="icon-btn sm" type="button" :aria-label="`Remove ${m.name} from ${group.name}`" @click="removeMember(group.id, m.id)"><Icon name="x" /></button>
        </div>
        <p v-if="members.length === 0" class="empty-state">No one yet.</p>
      </div>
    </div>

    <div v-if="last" class="toast glass strong" role="status">
      <Icon name="check" class="ok" />
      <span><b>{{ group.name }}</b> {{ last.held ? 'now holds' : 'no longer holds' }} <b>{{ last.role }}</b> on {{ last.label }}</span>
      <button class="btn ghost sm" type="button" @click="undo(last)">Undo</button>
    </div>

    <DialogScrim v-if="dialog === 'delete'" @close="dialog = null">
      <form class="dialog glass strong" role="dialog" aria-modal="true" aria-labelledby="g-dlg" @submit.prevent="remove">
        <h2 id="g-dlg">Delete {{ group.name }}?</h2>
        <p>{{ members.length }} people lose the access it gives them.</p>
        <div class="btn-row end"><button class="btn" type="button" @click="dialog = null">Cancel</button><button class="btn danger-solid" type="submit">Delete Group</button></div>
      </form>
    </DialogScrim>
    <DialogScrim v-if="dialog === 'rename'" @close="dialog = null">
      <form class="dialog glass strong" role="dialog" aria-modal="true" aria-labelledby="g-dlg" @submit.prevent="rename">
        <h2 id="g-dlg">Rename {{ group.name }}</h2>
        <div class="field"><label class="label" for="g-name">Name</label><input id="g-name" v-model="draft" class="input md" required maxlength="64" /></div>
        <div class="btn-row end"><button class="btn" type="button" @click="dialog = null">Cancel</button><button class="btn primary" type="submit">Save name</button></div>
      </form>
    </DialogScrim>
  </section>
</template>
