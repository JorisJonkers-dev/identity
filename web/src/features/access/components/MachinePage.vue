<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppIcon from '../../../shared/AppIcon.vue'
import AppShell from '../../../shared/AppShell.vue'
import BackLink from '../../../shared/BackLink.vue'
import DialogScrim from '../../../shared/DialogScrim.vue'
import Icon from '../../../shared/Icon'
import RoleChip from '../../../shared/RoleChip.vue'
import { useCopy } from '../../../shared/useCopy'
import type { MachinesOverview } from '../../../infrastructure/api/types.gen'
import { useMachines } from '../composables/useMachines'

const route = useRoute()
const router = useRouter()
const { overview, loadError, actionError, setRole, newSecret, deleteMachine } = useMachines()
const { copied, copy } = useCopy()
const machineId = computed(() => String(route.params.machineId))
const machine = computed(() => overview.value?.machines.find((m) => m.id === machineId.value))
const holds = (o: MachinesOverview, clientId: string, role: string) =>
  o.assignments.some((a) => a.machineId === machineId.value && a.clientId === clientId && a.role === role)
// Every app, those the Machine Identity holds a role on first.
const appsOf = (o: MachinesOverview) =>
  o.clients.map((c) => ({ client: c, held: c.roles.some((r) => holds(o, c.id, r)) })).sort((a, b) => Number(b.held) - Number(a.held))
const secret = ref('')
const deleting = ref(false)
function remove(id: string) {
  deleting.value = false
  deleteMachine(id, () => void router.replace('/admin/machines'))
}
</script>

<template>
  <AppShell admin>
    <BackLink to="/admin/machines" label="Machines" />
    <p v-if="loadError || actionError" role="alert" class="alert"><Icon name="alert" />{{ loadError ?? actionError }}</p>
    <template v-if="overview && machine">
      <section class="glass hero">
        <div class="hero-t">
          <h1>{{ machine.name }}</h1>
          <p>Signs in as the ServiceAccount <code>{{ machine.namespace }}/{{ machine.serviceAccount }}</code></p>
          <div class="pills">
            <span class="pill"><Icon name="key" />Client ID <code>{{ machine.id }}</code></span>
            <span class="pill"><Icon name="apps" />{{ appsOf(overview).filter((a) => a.held).length }} apps</span>
          </div>
        </div>
      </section>
      <section class="glass group" style="margin-top: 16px">
        <div class="group-h"><h2>Access</h2><span class="soft">Click a role to give it to {{ machine.name }}. Click again to take it away.</span></div>
        <div class="acc">
          <div v-for="a in appsOf(overview)" :key="a.client.id" :class="['acc-row', { held: a.held }]">
            <AppIcon :label="a.client.label" size="sm" />
            <span class="acc-n"><b>{{ a.client.label }}</b><small>{{ a.held ? 'Has access' : 'No access' }}</small></span>
            <div class="chips" role="group" :aria-label="`${machine.name} on ${a.client.label}`">
              <RoleChip v-for="r in a.client.roles" :key="r" :role="r" :held="holds(overview, a.client.id, r)" @toggle="setRole(machine.id, a.client.id, r, !holds(overview, a.client.id, r))" />
            </div>
          </div>
        </div>
      </section>
      <section class="glass group" style="margin-top: 16px">
        <div class="row">
          <span class="row-ic"><Icon name="lock" /></span>
          <span class="grow"><b>Client secret</b><small>Only for a workload outside the cluster. The old secret works until the new one is first used.</small></span>
          <button class="btn sm" type="button" @click="newSecret(machine.id, (s) => (secret = s))">Make a new secret</button>
        </div>
      </section>
      <section class="glass group" style="margin-top: 16px">
        <div class="row">
          <span class="row-ic danger-t"><Icon name="trash" /></span>
          <span class="grow"><b>Delete Machine Identity</b><small>Its roles and secret go with it. Tokens it holds end within 15 minutes.</small></span>
          <button class="btn sm danger-t" type="button" @click="deleting = true">Delete</button>
        </div>
      </section>
      <DialogScrim v-if="secret" @close="secret = ''">
        <div class="dialog glass strong" role="dialog" aria-modal="true" aria-labelledby="mc-secret">
          <h2 id="mc-secret">New secret for {{ machine.name }}</h2>
          <p class="warn-line"><Icon name="alert" />Shown once.</p>
          <div class="secret"><code>{{ secret }}</code><button class="btn sm primary" type="button" @click="copy(secret)"><Icon name="copy" />{{ copied ? 'Copied' : 'Copy' }}</button></div>
          <div class="btn-row end"><button class="btn" type="button" @click="secret = ''">Done</button></div>
        </div>
      </DialogScrim>
      <DialogScrim v-if="deleting" @close="deleting = false">
        <form class="dialog glass strong" role="dialog" aria-modal="true" aria-labelledby="mc-dlg" @submit.prevent="remove(machine.id)">
          <h2 id="mc-dlg">Delete {{ machine.name }}?</h2>
          <p>It can no longer sign in. Tokens it already holds end within 15 minutes. This cannot be undone.</p>
          <div class="btn-row end"><button class="btn" type="button" @click="deleting = false">Cancel</button><button class="btn danger-solid" type="submit">Delete {{ machine.name }}</button></div>
        </form>
      </DialogScrim>
    </template>
  </AppShell>
</template>
