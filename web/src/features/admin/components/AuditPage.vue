<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { AuditEvent, AuditKind } from '../../../infrastructure/api/types.gen'
import { type AuditFilter, useAudit } from '../composables/useAudit'
import { clock, dayOf, describeEvent, tone } from '../format'
import AdminLayout from './AdminLayout.vue'

const kinds: AuditKind[] = ['sign-in', 'people', 'access', 'tokens']

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

const { events, isLoading, loadError, hasOlder, loadingOlder, older, users, clients } = useAudit(filter)
const now = new Date()

/** Each event, and the day it starts when it is the first of that day. */
const rows = computed(() =>
  (events.value ?? []).map((e: AuditEvent, i: number, all: AuditEvent[]) => {
    const day = dayOf(e.at, now)
    const previous = all[i - 1]
    return { e, day: previous && dayOf(previous.at, now) === day ? null : day }
  }),
)
</script>

<template>
  <AdminLayout>
    <div class="head">
      <div>
        <h1>Audit</h1>
        <p class="sub">Kept here for one year. Every event is also sent to Loki.</p>
      </div>
    </div>
    <div class="filters" role="group" aria-label="Filter the events">
      <label class="pill" :class="{ a: filter.user === '' }">
        <span class="hidden">User</span>
        <select :value="filter.user" @change="narrow({ user: ($event.target as HTMLSelectElement).value })">
          <option value="">All Users</option>
          <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
        </select>
      </label>
      <label class="pill" :class="{ a: filter.client === '' }">
        <span class="hidden">App</span>
        <select :value="filter.client" @change="narrow({ client: ($event.target as HTMLSelectElement).value })">
          <option value="">All Clients</option>
          <option v-for="c in clients" :key="c.id" :value="c.id">{{ c.label }}</option>
        </select>
      </label>
      <button
        v-for="k in kinds"
        :key="k"
        type="button"
        class="pill"
        :class="{ a: filter.kind === k }"
        :aria-pressed="filter.kind === k"
        @click="narrow({ kind: filter.kind === k ? '' : k })"
      >
        {{ k }}
      </button>
    </div>
    <div class="panel">
      <p v-if="loadError" role="alert" class="error">{{ loadError }}</p>
      <ol v-else-if="rows.length > 0" class="audit" aria-label="Audit Events, newest first">
        <template v-for="{ e, day } in rows" :key="e.id">
          <li v-if="day" class="day">{{ day }}</li>
          <li>
            <time :datetime="e.at">{{ clock(e.at) }}</time>
            <span class="k" :class="tone(e)" aria-hidden="true"></span>
            <p>
              <!-- Vue drops the line breaks between these; each part carries its own spaces. -->
              <template v-for="(part, i) in describeEvent(e)" :key="i">
                <b v-if="part.as === 'name'">{{ part.text }}</b>
                <span v-else-if="part.as === 'pill'" class="pill">{{ part.text }}</span>
                <template v-else>{{ part.text }}</template>
              </template>
            </p>
            <span class="pill">{{ e.kind }}</span>
          </li>
        </template>
      </ol>
      <p v-else-if="isLoading" class="muted">Loading…</p>
      <p v-else class="muted">No Audit Events match.</p>
      <button v-if="hasOlder" type="button" class="btn older" :disabled="loadingOlder" @click="older">
        {{ loadingOlder ? 'Loading…' : 'Show older Audit Events' }}
      </button>
    </div>
  </AdminLayout>
</template>

<style scoped>
.filters {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
button.pill {
  cursor: pointer;
}
.pill select {
  appearance: none;
  font: inherit;
  color: inherit;
  background: transparent;
  border: 0;
  padding: 0;
  cursor: pointer;
  max-width: 14rem;
}
.hidden {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip-path: inset(50%);
  white-space: nowrap;
}
.audit {
  display: grid;
  margin: 0;
  padding: 0;
  list-style: none;
}
.audit > li {
  display: grid;
  grid-template-columns: 74px 22px minmax(0, 1fr) auto;
  gap: 10px;
  align-items: center;
  padding: 9px 0;
  border-top: 1px solid var(--line);
}
.audit > li:first-child {
  border-top: 0;
}
.audit > li.day {
  display: block;
  font: 500 10.5px var(--mono);
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--ink-3);
  padding: 14px 0 4px;
}
.audit > li.day:first-child {
  padding-top: 0;
}
.audit time {
  font: 11.5px var(--mono);
  color: var(--ink-3);
  font-variant-numeric: tabular-nums;
}
.audit .k {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--ink-3);
  justify-self: center;
}
.audit .k.ok {
  background: var(--ok);
}
.audit .k.bad {
  background: var(--bad);
}
.audit .k.a {
  background: var(--accent);
}
.audit p {
  margin: 0;
  min-width: 0;
  font-size: 13px;
  overflow-wrap: anywhere;
}
.older {
  margin-top: 12px;
}
@media (max-width: 520px) {
  .audit > li {
    grid-template-columns: 64px 14px minmax(0, 1fr);
  }
  .audit > li > .pill {
    display: none;
  }
}
</style>
