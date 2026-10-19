<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import AppIcon from '../../../shared/AppIcon.vue'
import AppShell from '../../../shared/AppShell.vue'
import PersonAvatar from '../../../shared/PersonAvatar.vue'
import BackLink from '../../../shared/BackLink.vue'
import GlassSelect from '../../../shared/GlassSelect.vue'
import Icon from '../../../shared/Icon'
import RoleChip from '../../../shared/RoleChip.vue'
import UserTokens from '../../tokens/components/UserTokens.vue'
import type { AccessOverview } from '../../../infrastructure/api/types.gen'
import { useAccess } from '../composables/useAccess'
import { accessOf, groupsOf, holdsDirect } from '../model'

const route = useRoute()
const { overview, loadError, actionError, addMember, removeMember, signOutUser, setUserRole } = useAccess()
const userId = computed(() => String(route.params.userId))
const person = computed(() => overview.value?.users.find((u) => u.id === userId.value))
const groupsFor = (o: AccessOverview) => groupsOf(o, userId.value)
const addable = (o: AccessOverview) => o.groups.filter((g) => !g.members.includes(userId.value)).map((g) => ({ value: g.id, label: g.name }))
// Every app, those the person holds a role on first: Group roles locked, direct ones to toggle.
function accessFor(o: AccessOverview) {
  const map = accessOf(o, userId.value)
  return o.clients
    .map((c) => {
      const viaGroups = map.get(c.id) ?? new Map<string, string[]>()
      const direct = c.roles.filter((r) => holdsDirect(o, userId.value, c.id, r))
      return { client: c, viaGroups, direct, held: viaGroups.size > 0 || direct.length > 0 }
    })
    .sort((a, b) => Number(b.held) - Number(a.held))
}
const signedOut = ref(false)
function source(a: { viaGroups: Map<string, string[]>; direct: string[] }): string {
  const groups = [...new Set([...a.viaGroups.values()].flat())]
  if (a.direct.length) return groups.length ? `Direct, also via ${groups.join(' · ')}` : 'Direct'
  return groups.length ? `Via ${groups.join(' · ')}` : 'No access'
}
function chipTitle(groups: string[] | undefined, direct: boolean): string | undefined {
  if (!groups) return undefined
  return direct ? `Also via ${groups.join(', ')}` : `From ${groups.join(', ')}. Change it on the Group.`
}
</script>

<template>
  <AppShell admin>
    <BackLink to="/admin/people" label="People" />
    <p v-if="loadError || actionError" role="alert" class="alert"><Icon name="alert" />{{ loadError ?? actionError }}</p>
    <template v-if="overview && person">
      <section class="glass hero">
        <div class="hero-av"><PersonAvatar :name="person.name" size="xxl" /></div>
        <div class="hero-t">
          <h1>{{ person.name }}</h1>
          <p>{{ person.email }}</p>
          <div class="pills">
            <span v-if="groupsFor(overview).some((g) => g.admins)" class="pill accent"><Icon name="shield" />Admin</span>
            <span class="pill"><Icon name="users" />{{ groupsFor(overview).length }} Groups</span>
            <span class="pill"><Icon name="apps" />{{ accessFor(overview).filter((a) => a.held).length }} apps</span>
          </div>
        </div>
      </section>
      <section class="glass group">
        <div class="group-h"><h2>Groups</h2></div>
        <div class="gchips">
          <span v-for="g in groupsFor(overview)" :key="g.id" class="gchip">{{ g.name }}<button type="button" :aria-label="`Remove ${person.name} from ${g.name}`" @click="removeMember(g.id, person.id)"><Icon name="x" /></button></span>
          <span v-if="groupsFor(overview).length === 0" class="soft">No Group yet</span>
          <GlassSelect :options="addable(overview)" :label="`Add ${person.name} to a Group`" placeholder="Add to a Group" icon="plus" small @pick="(g) => addMember(g, person!.id)" />
        </div>
      </section>
      <section class="glass group" style="margin-top: 16px">
        <div class="group-h"><h2>Access</h2><span class="soft">Click a role to give it to {{ person.name }} directly. Click again to take it away.</span></div>
        <div class="acc">
          <div v-for="a in accessFor(overview)" :key="a.client.id" :class="['acc-row', { held: a.held }]">
            <AppIcon :label="a.client.label" size="sm" />
            <span class="acc-n"><b>{{ a.client.label }}</b><small>{{ source(a) }}</small></span>
            <div class="chips" role="group" :aria-label="`${person.name} on ${a.client.label}`">
              <RoleChip
                v-for="r in a.client.roles"
                :key="r"
                :role="r"
                :held="a.direct.includes(r) || a.viaGroups.has(r)"
                :locked="a.viaGroups.has(r) && !a.direct.includes(r)"
                :title="chipTitle(a.viaGroups.get(r), a.direct.includes(r))"
                @toggle="setUserRole(person.id, a.client.id, r, !a.direct.includes(r))"
              />
            </div>
          </div>
        </div>
      </section>
      <div class="cols-2" style="margin-top: 16px">
        <section class="glass group">
          <div class="group-h"><h2>Sessions</h2></div>
          <div class="row">
            <span class="row-ic"><Icon name="logout" /></span>
            <span class="grow"><b>Sign out everywhere</b><small>{{ signedOut ? 'Signed out on every device' : 'Ends every session' }}</small></span>
            <button class="btn sm" type="button" :disabled="signedOut" @click="signOutUser(person.id, () => (signedOut = true))">Sign out everywhere</button>
          </div>
        </section>
        <section class="glass group">
          <div class="group-h"><h2>Tokens</h2></div>
          <div class="row"><UserTokens :user-id="person.id" :name="person.name" /></div>
        </section>
      </div>
    </template>
  </AppShell>
</template>
