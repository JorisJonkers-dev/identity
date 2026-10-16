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

// A filter that narrows nothing reads as the canvas's highlighted pill.
const on = 'border-transparent bg-accent-soft text-accent'
const dot = { ok: 'bg-ok', bad: 'bg-bad', a: 'bg-accent', '': 'bg-ink-3' }
const select = 'max-w-56 cursor-pointer appearance-none border-0 bg-transparent p-0 [font:inherit] text-inherit [field-sizing:content]'

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
    <div>
      <h1 class="m-0 font-display text-[22px]/[1.15] font-bold tracking-[-0.02em] text-balance">Audit</h1>
      <p class="mt-0.5 mb-0 text-[13px] text-ink-3">Kept here for one year. Every event is also sent to Loki.</p>
    </div>
    <div class="flex flex-wrap gap-1.5" role="group" aria-label="Filter the events">
      <label class="pill" :class="filter.user === '' && on">
        <span class="sr-only">User</span>
        <select :class="select" :value="filter.user" @change="narrow({ user: ($event.target as HTMLSelectElement).value })">
          <option value="">All Users</option>
          <option v-for="u in users" :key="u.id" :value="u.id">{{ u.name }}</option>
        </select>
      </label>
      <label class="pill" :class="filter.client === '' && on">
        <span class="sr-only">App</span>
        <select :class="select" :value="filter.client" @change="narrow({ client: ($event.target as HTMLSelectElement).value })">
          <option value="">All Clients</option>
          <option v-for="c in clients" :key="c.id" :value="c.id">{{ c.label }}</option>
        </select>
      </label>
      <button
        v-for="k in kinds"
        :key="k"
        type="button"
        class="pill cursor-pointer"
        :class="filter.kind === k && on"
        :aria-pressed="filter.kind === k"
        @click="narrow({ kind: filter.kind === k ? '' : k })"
      >
        {{ k }}
      </button>
    </div>
    <div class="panel">
      <p v-if="loadError" role="alert" class="m-0 text-bad">{{ loadError }}</p>
      <ol v-else-if="rows.length > 0" class="m-0 grid list-none p-0" aria-label="Audit Events, newest first">
        <template v-for="{ e, day } in rows" :key="e.id">
          <li v-if="day" class="eyebrow pt-3.5 pb-1 first:pt-0" data-day>{{ day }}</li>
          <li
            class="grid grid-cols-[74px_22px_minmax(0,1fr)_auto] items-center gap-2.5 border-t border-line py-[9px] first:border-t-0 max-[520px]:grid-cols-[64px_14px_minmax(0,1fr)]"
            data-event
          >
            <time class="font-mono text-[11.5px] text-ink-3 tabular-nums" :datetime="e.at">{{ clock(e.at) }}</time>
            <span class="size-2 justify-self-center rounded-full" :class="dot[tone(e)]" :data-tone="tone(e)" aria-hidden="true"></span>
            <p class="m-0 min-w-0 text-[13px] wrap-anywhere">
              <!-- Vue drops the line breaks between these; each part carries its own spaces. -->
              <template v-for="(part, i) in describeEvent(e)" :key="i">
                <b v-if="part.as === 'name'">{{ part.text }}</b>
                <span v-else-if="part.as === 'pill'" class="pill" data-role>{{ part.text }}</span>
                <template v-else>{{ part.text }}</template>
              </template>
            </p>
            <span class="pill max-[520px]:hidden" data-kind>{{ e.kind }}</span>
          </li>
        </template>
      </ol>
      <p v-else-if="isLoading" class="m-0 text-ink-3">Loading…</p>
      <p v-else class="m-0 text-ink-3">No Audit Events match.</p>
      <button v-if="hasOlder" type="button" class="btn mt-3" :disabled="loadingOlder" @click="older">
        {{ loadingOlder ? 'Loading…' : 'Show older Audit Events' }}
      </button>
    </div>
  </AdminLayout>
</template>
