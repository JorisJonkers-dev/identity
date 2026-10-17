<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query'
import { computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import { getDashboardOptions, getProfileOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import SideLayout from '../../../shared/SideLayout.vue'
import type { SideSection } from '../../../shared/side'

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
const sections = computed<SideSection[]>(() => [
  {
    title: 'Admin',
    links: [
      { to: '/admin', label: 'Dashboard' },
      { to: '/access#waiting-title', label: 'Unassigned', count: dashboard.data.value?.unassigned.length },
      { to: '/access', label: 'Groups and access' },
      { to: '/admin/audit', label: 'Audit' },
    ],
  },
  { title: 'You', links: [{ to: '/', label: 'Your apps' }] },
])
</script>

<template>
  <SideLayout label="Admin" :name="profile.data.value?.name" detail="Admin · 12h session" :sections="sections">
    <slot />
  </SideLayout>
</template>
