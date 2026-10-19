<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import AppShell from '../../../shared/AppShell.vue'
import Icon from '../../../shared/Icon'
import { useMachines } from '../composables/useMachines'

const router = useRouter()
const { overview, loadError, actionError, createMachine } = useMachines()
const adding = ref(false)
const name = ref('')
const namespace = ref('')
const serviceAccount = ref('')
// Each Machine Identity with how many App Roles it holds; none while loading.
const machines = computed(() => {
  const o = overview.value
  if (!o) return []
  return o.machines.map((m) => ({ ...m, roles: o.assignments.filter((a) => a.machineId === m.id).length }))
})
function add() {
  createMachine({ name: name.value, namespace: namespace.value, serviceAccount: serviceAccount.value }, (m) => {
    adding.value = false
    void router.push(`/admin/machines/${m.id}`)
  })
}
</script>

<template>
  <AppShell admin>
    <header class="head">
      <h1>Machines</h1>
      <div class="end">
        <span class="quiet">{{ machines.length }} {{ machines.length === 1 ? 'Machine Identity' : 'Machine Identities' }}</span>
        <button class="btn sm primary" type="button" @click="adding = true"><Icon name="plus" />Add Machine Identity</button>
      </div>
    </header>
    <p v-if="loadError || actionError" role="alert" class="alert"><Icon name="alert" />{{ loadError ?? actionError }}</p>
    <form v-if="adding" class="glass group mc-add" @submit.prevent="add">
      <div class="group-h"><h2>New Machine Identity</h2><span class="soft">A workload signs in as the Kubernetes ServiceAccount it runs as.</span></div>
      <div class="mc-fields">
        <label class="label" for="mc-name">Name</label><input id="mc-name" v-model="name" class="input md" required maxlength="64" />
        <label class="label" for="mc-ns">Namespace</label><input id="mc-ns" v-model="namespace" class="input md" required maxlength="63" autocapitalize="off" spellcheck="false" />
        <label class="label" for="mc-sa">ServiceAccount</label><input id="mc-sa" v-model="serviceAccount" class="input md" required maxlength="253" autocapitalize="off" spellcheck="false" />
      </div>
      <div class="btn-row end"><button class="btn sm ghost" type="button" @click="adding = false">Cancel</button><button class="btn sm primary" type="submit">Add</button></div>
    </form>
    <div class="glass lt">
      <div class="lt-h cols-machines"><span>Name</span><span>ServiceAccount</span><span>Roles</span><span /></div>
      <button v-for="m in machines" :key="m.id" type="button" class="lt-r cols-machines row-btn" @click="router.push(`/admin/machines/${m.id}`)">
        <span class="app-c"><Icon name="bot" />{{ m.name }}</span>
        <span class="muted">{{ m.namespace }}/{{ m.serviceAccount }}</span>
        <span class="muted">{{ m.roles }} {{ m.roles === 1 ? 'role' : 'roles' }}</span>
        <Icon name="chevr" class="chevr" />
      </button>
      <p v-if="overview && machines.length === 0" class="empty-state">No Machine Identities yet. Add one for each workload that calls an app.</p>
    </div>
  </AppShell>
</template>
