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
import { accessOf, groupsOf } from '../model'

const route = useRoute()
const { overview, loadError, actionError, addMember, removeMember, signOutUser } = useAccess()
const userId = computed(() => String(route.params.userId))
const person = computed(() => overview.value?.users.find((u) => u.id === userId.value))
const groupsFor = (o: AccessOverview) => groupsOf(o, userId.value)
const addable = (o: AccessOverview) => o.groups.filter((g) => !g.members.includes(userId.value)).map((g) => ({ value: g.id, label: g.name }))
function accessFor(o: AccessOverview) {
  const map = accessOf(o, userId.value)
  return o.clients.flatMap((c) => {
    const roles = map.get(c.id)
    return roles ? [{ client: c, roles }] : []
  })
}
const signedOut = ref(false)
function via(roles: Map<string, string[]>): string {
  return `via ${[...new Set([...roles.values()].flat())].join(' · ')}`
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
            <span class="pill"><Icon name="apps" />{{ accessFor(overview).length }} apps</span>
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
        <div class="group-h"><h2>Access</h2><span class="soft">through Groups</span></div>
        <div class="acc">
          <div v-for="a in accessFor(overview)" :key="a.client.id" class="acc-row held">
            <AppIcon :label="a.client.label" size="sm" />
            <span class="acc-n"><b>{{ a.client.label }}</b><small>{{ via(a.roles) }}</small></span>
            <div class="chips">
              <RoleChip v-for="r in a.client.roles" :key="r" :role="r" :held="a.roles.has(r)" :locked="a.roles.has(r)" :title="a.roles.has(r) ? `via ${a.roles.get(r)!.join(', ')}` : undefined" />
            </div>
          </div>
          <p v-if="accessFor(overview).length === 0" class="empty-state">No apps yet. Add a Group above.</p>
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
