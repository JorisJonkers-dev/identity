<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query'
import { reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { getProfileOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { AccessOverview } from '../../../infrastructure/api/types.gen'
import { useAccess } from '../composables/useAccess'

// An Admin whose session proved no Second Factor is sent to add one first (docs/adr/0005).
const router = useRouter()
const profile = useQuery({ ...getProfileOptions(), retry: false })
watch(
  () => profile.data.value,
  (p) => {
    if (p?.admin && !p.secondFactor) void router.replace({ path: '/security', query: { for: 'admin' } })
  },
  { immediate: true },
)

const access = useAccess()
const { overview, isLoading, loadError, actionError } = access
const newGroup = ref('')
const names = reactive<Record<string, string>>({})
const picked = reactive<Record<string, string>>({})
const placing = reactive<Record<string, string>>({})

function holds(o: AccessOverview, groupId: string, clientId: string, role: string): boolean {
  return o.assignments.some((a) => a.groupId === groupId && a.clientId === clientId && a.role === role)
}

function person(o: AccessOverview, id: string): string {
  const u = o.users.find((x) => x.id === id)
  return u ? `${u.name} (${u.email})` : id
}

function outsiders(o: AccessOverview, members: string[]) {
  return o.users.filter((u) => !members.includes(u.id))
}

/** Users in no Group, who can sign in but open no app (docs/adr/0005). */
function unassigned(o: AccessOverview) {
  const placed = new Set(o.groups.flatMap((g) => g.members))
  return o.users.filter((u) => !placed.has(u.id))
}

function place(userId: string) {
  const groupId = placing[userId]
  if (groupId) access.addMember(groupId, userId)
}

function toggle(groupId: string, clientId: string, role: string, event: Event) {
  access.setRole(groupId, clientId, role, (event.target as HTMLInputElement).checked)
}

function create() {
  access.createGroup(newGroup.value, () => (newGroup.value = ''))
}

function add(groupId: string) {
  const userId = picked[groupId]
  if (userId) access.addMember(groupId, userId)
}
</script>

<template>
  <section aria-labelledby="access-title">
    <h1 id="access-title">Access</h1>
    <p v-if="isLoading">Loading…</p>
    <p v-else-if="loadError" role="alert" class="error">{{ loadError }}</p>
    <template v-else-if="overview">
      <p>
        Select a role to give it to everyone in that Group. A person in several Groups gets the roles of all of them. Members of the
        admins Group manage this page, and open apps only through their other Groups.
      </p>
      <p v-if="actionError" role="alert" class="error">{{ actionError }}</p>

      <template v-if="unassigned(overview).length > 0">
        <h2 id="waiting-title">People waiting for a Group</h2>
        <p>These people confirmed their email. They can't open any app until you add them to a Group.</p>
        <ul aria-labelledby="waiting-title">
          <li v-for="u in unassigned(overview)" :key="u.id">
            <form class="inline" @submit.prevent="place(u.id)">
              <span>{{ u.name }} ({{ u.email }})</span>
              <label :for="`place-${u.id}`">Group</label>
              <select :id="`place-${u.id}`" v-model="placing[u.id]">
                <option v-for="g in overview.groups" :key="g.id" :value="g.id">{{ g.name }}</option>
              </select>
              <button type="submit">Add to Group</button>
            </form>
          </li>
        </ul>
      </template>

      <h2 id="matrix-title">App Roles</h2>
      <p v-if="overview.clients.length === 0">No apps are registered yet.</p>
      <div v-else class="scroll">
        <table aria-labelledby="matrix-title">
          <thead>
            <tr>
              <th scope="col">Group</th>
              <th v-for="client in overview.clients" :key="client.id" scope="col">{{ client.label }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="group in overview.groups.filter((g) => !g.admins)" :key="group.id">
              <th scope="row">{{ group.name }}</th>
              <td v-for="client in overview.clients" :key="client.id">
                <label v-for="role in client.roles" :key="role" class="role">
                  <input
                    type="checkbox"
                    :checked="holds(overview, group.id, client.id, role)"
                    :aria-label="`${group.name}: ${role} on ${client.label}`"
                    @change="toggle(group.id, client.id, role, $event)"
                  />
                  {{ role }}
                </label>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <h2>Groups</h2>
      <form class="inline" @submit.prevent="create">
        <label for="new-group">Group name</label>
        <input id="new-group" v-model="newGroup" required maxlength="64" />
        <button type="submit">Create Group</button>
      </form>

      <article v-for="group in overview.groups" :key="group.id" :aria-label="group.name">
        <h3>{{ group.name }}<span v-if="group.admins"> (Admins)</span></h3>
        <form class="inline" @submit.prevent="access.renameGroup(group.id, names[group.id] ?? '')">
          <label :for="`rename-${group.id}`">Rename to</label>
          <input :id="`rename-${group.id}`" v-model="names[group.id]" required maxlength="64" />
          <button type="submit">Rename</button>
        </form>
        <ul>
          <li v-for="member in group.members" :key="member">
            {{ person(overview, member) }}
            <button type="button" @click="access.removeMember(group.id, member)">Remove from Group</button>
          </li>
        </ul>
        <form class="inline" @submit.prevent="add(group.id)">
          <label :for="`add-${group.id}`">Add a member</label>
          <select :id="`add-${group.id}`" v-model="picked[group.id]">
            <option v-for="u in outsiders(overview, group.members)" :key="u.id" :value="u.id">{{ u.name }} ({{ u.email }})</option>
          </select>
          <button type="submit">Add</button>
        </form>
        <button v-if="!group.admins" type="button" @click="access.deleteGroup(group.id)">Delete {{ group.name }}</button>
      </article>
    </template>
  </section>
</template>

<style scoped>
.error {
  color: #a4161a;
}
.scroll {
  overflow-x: auto;
}
table {
  border-collapse: collapse;
}
th,
td {
  border: 1px solid #c8c8c8;
  padding: 0.25rem 0.5rem;
  text-align: left;
  vertical-align: top;
}
.role {
  display: block;
  white-space: nowrap;
}
.inline {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  align-items: center;
}
/* An option's text is a person's name and email, which can be wider than a phone. */
.inline > * {
  min-width: 0;
  max-width: 100%;
  overflow-wrap: anywhere;
}
article {
  border-top: 1px solid #c8c8c8;
  margin-top: 1rem;
}
</style>
