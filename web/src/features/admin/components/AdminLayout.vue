<script setup lang="ts">
import '@fontsource-variable/bricolage-grotesque/opsz.css'
import '@fontsource-variable/geist'
import '@fontsource-variable/geist-mono'
import { useQuery } from '@tanstack/vue-query'
import { computed, watch } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { getDashboardOptions, getProfileOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { initials } from '../format'

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
const dashboard = useQuery({ ...getDashboardOptions(), retry: false })
const waiting = computed(() => dashboard.data.value?.unassigned.length ?? 0)

const link =
  'flex items-center justify-between rounded-lg px-2.5 py-[7px] text-[13px] font-medium text-ink-2 no-underline max-[720px]:flex-none max-[720px]:gap-1.5'
const open = 'bg-accent-soft text-accent!'
const section = 'eyebrow px-2.5 pt-3.5 pb-1 max-[720px]:hidden'
</script>

<template>
  <div
    class="grid min-h-dvh grid-cols-[200px_minmax(0,1fr)] bg-surface-2 font-body text-[14px]/[1.5] text-ink antialiased [color-scheme:light] max-[720px]:grid-cols-1 dark:[color-scheme:dark] [&_*]:box-border [&_:focus-visible]:rounded-md [&_:focus-visible]:outline-2 [&_:focus-visible]:outline-offset-2 [&_:focus-visible]:outline-accent"
  >
    <nav
      class="flex flex-col gap-0.5 border-r border-line bg-surface px-3 py-4 max-[720px]:flex-row max-[720px]:items-center max-[720px]:overflow-x-auto max-[720px]:border-r-0 max-[720px]:border-b max-[720px]:px-[max(16px,env(safe-area-inset-left,0px))] max-[720px]:py-2.5"
      aria-label="Admin"
    >
      <div v-if="profile.data.value" data-who class="flex items-center gap-2.5 px-2 pt-1.5 pb-4 max-[720px]:hidden">
        <span class="grid size-[30px] flex-none place-items-center rounded-full bg-accent-soft text-xs font-semibold text-accent" aria-hidden="true">
          {{ initials(profile.data.value.name) }}
        </span>
        <div>
          <b>{{ profile.data.value.name }}</b>
          <small class="block text-[11.5px] text-ink-3">Admin · 12h session</small>
        </div>
      </div>
      <span :class="section">Admin</span>
      <RouterLink to="/admin" :class="link" :exact-active-class="open">Dashboard</RouterLink>
      <RouterLink to="/access#waiting-title" :class="link">
        Unassigned
        <span v-if="waiting > 0" data-count class="rounded-full bg-warn-soft px-[7px] py-px font-mono text-[11px] font-medium text-warn">{{ waiting }}</span>
      </RouterLink>
      <RouterLink to="/access" :class="link">Groups and access</RouterLink>
      <RouterLink to="/admin/audit" :class="link" :exact-active-class="open">Audit</RouterLink>
      <span :class="section">You</span>
      <RouterLink to="/" :class="link">Your apps</RouterLink>
    </nav>
    <div
      class="grid min-w-0 content-start gap-[18px] px-6 py-[22px] max-[720px]:py-[18px] max-[720px]:pr-[max(16px,env(safe-area-inset-right,0px))] max-[720px]:pl-[max(16px,env(safe-area-inset-left,0px))]"
    >
      <slot />
    </div>
  </div>
</template>
