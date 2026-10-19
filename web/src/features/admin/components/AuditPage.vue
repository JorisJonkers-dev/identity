<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { AuditEvent, AuditKind } from '../../../infrastructure/api/types.gen'
import AppShell from '../../../shared/AppShell.vue'
import GlassSelect from '../../../shared/GlassSelect.vue'
import Icon from '../../../shared/Icon'
import { type AuditFilter, useAudit } from '../composables/useAudit'
import { clock, dayOf, describeEvent } from '../format'

const kinds: AuditKind[] = ['sign-in', 'people', 'access', 'tokens']
const kindLabel: Record<AuditKind, string> = { 'sign-in': 'Sign-in', people: 'People', access: 'Access', tokens: 'Tokens' }

// The filter lives in the address, so a filtered view can be shared or reloaded (/admin/audit?kind=access).
const route = useRoute()
const router = useRouter()
const param = (name: string) => (typeof route.query[name] === 'string' ? route.query[name] : '')
const filter = computed<AuditFilter>(() => ({
  user: param('user'),
  client: param('client'),
  kind: kinds.find((k) => k === param('kind')) ?? '',
}))
function narrow(change: Partial<AuditFilter>) {
  const next = { ...filter.value, ...change }
  void router.replace({ query: Object.fromEntries(Object.entries(next).filter(([, v]) => v !== '')) })
}

const { events, loadError, hasOlder, loadingOlder, older, users, clients } = useAudit(filter)
const now = new Date()
const userOptions = computed(() => [{ value: '', label: 'Everyone' }, ...users.value.map((u) => ({ value: u.id, label: u.name }))])
const clientOptions = computed(() => [{ value: '', label: 'All apps' }, ...clients.value.map((c) => ({ value: c.id, label: c.label }))])
const user = computed({ get: () => filter.value.user, set: (v: string) => {
  narrow({ user: v })
} })
const client = computed({ get: () => filter.value.client, set: (v: string) => {
  narrow({ client: v })
} })

/** The events, a day at a time. */
const days = computed(() => {
  const out: { day: string; events: AuditEvent[] }[] = []
  for (const e of events.value ?? []) {
    const day = dayOf(e.at, now)
    const last = out.at(-1)
    if (last?.day === day) last.events.push(e)
    else out.push({ day, events: [e] })
  }
  return out
})
</script>

<template>
  <AppShell admin>
    <header class="head"><h1>Audit</h1></header>
    <div class="tools bar-tools glass">
      <GlassSelect v-model="user" :options="userOptions" label="Person" icon="user" />
      <GlassSelect v-model="client" :options="clientOptions" label="App" icon="apps" />
      <div class="seg" role="group" aria-label="Kind">
        <button type="button" :aria-pressed="filter.kind === ''" @click="narrow({ kind: '' })">All</button>
        <button v-for="k in kinds" :key="k" type="button" :aria-pressed="filter.kind === k" @click="narrow({ kind: k })">{{ kindLabel[k] }}</button>
      </div>
    </div>
    <p v-if="loadError" role="alert" class="alert"><Icon name="alert" />{{ loadError }}</p>
    <div v-if="days.length" class="glass audit-list">
      <template v-for="d in days" :key="d.day">
        <div class="day"><h2>{{ d.day }}</h2></div>
        <div v-for="e in d.events" :key="e.id" :class="['ev', { fail: e.action === 'sign-in-failed' }]">
          <time :datetime="e.at">{{ clock(e.at).slice(0, 5) }}</time>
          <span :class="['tag', { bad: e.action === 'sign-in-failed' }]">{{ kindLabel[e.kind] }}</span>
          <p><template v-for="(p, i) in describeEvent(e)" :key="i"><b v-if="p.as === 'name'">{{ p.text }}</b><code v-else-if="p.as === 'pill'">{{ p.text }}</code><template v-else>{{ p.text }}</template></template></p>
        </div>
      </template>
      <div v-if="hasOlder" class="more"><button class="btn sm" type="button" :disabled="loadingOlder" @click="older">Load more</button></div>
    </div>
    <div v-else-if="events" class="glass empty"><p>No events match.</p><button class="btn sm" type="button" @click="router.replace({ query: {} })">Clear filters</button></div>
  </AppShell>
</template>
