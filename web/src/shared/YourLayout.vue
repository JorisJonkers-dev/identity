<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query'
import { computed } from 'vue'
import { getProfileOptions } from '../infrastructure/api/@tanstack/vue-query.gen'
import SideLayout from './SideLayout.vue'
import type { SideSection } from './side'

// The pages a User has of their own, as the canvas drew them (B1 to B3); an Admin also sees theirs.
const profile = useQuery({ ...getProfileOptions(), retry: false })
const sections = computed<SideSection[]>(() => {
  // The canvas puts a User's own pages first, under no heading.
  const yours: SideSection = {
    links: [
      { to: '/', label: 'Apps' },
      { to: '/profile', label: 'Profile' },
      { to: '/security', label: 'Security' },
      { to: '/profile/tokens', label: 'Tokens' },
    ],
  }
  return profile.data.value?.admin ? [yours, { title: 'Admin', links: [{ to: '/admin', label: 'Dashboard' }] }] : [yours]
})
</script>

<template>
  <SideLayout label="Your pages" :name="profile.data.value?.name" :detail="profile.data.value?.email" :sections="sections">
    <slot />
  </SideLayout>
</template>
