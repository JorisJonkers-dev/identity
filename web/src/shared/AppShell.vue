<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query'
import { computed, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { getDashboardOptions, getProfileOptions, signOutMutation } from '../infrastructure/api/@tanstack/vue-query.gen'
import { zProblem } from '../infrastructure/api/zod.gen'
import { navigation } from '../infrastructure/navigation'
import { useAction } from '../infrastructure/useAction'
import PersonAvatar from './PersonAvatar.vue'
import Icon from './Icon'

/** Every signed-in page: one centred bar, a Personal or Admin view, and the page below it. */
const props = defineProps<{ admin?: boolean; narrow?: boolean }>()
const route = useRoute()
const router = useRouter()
const profile = useQuery({ ...getProfileOptions(), retry: false })
const me = computed(() => profile.data.value)
const dashboard = useQuery(computed(() => ({ ...getDashboardOptions(), retry: false, enabled: me.value?.admin === true })))
const waiting = computed(() => dashboard.data.value?.unassigned.length ?? 0)

// A signed-in page with nobody signed in sends them to sign in.
watch(
  () => zProblem.safeParse(profile.error.value).data?.status,
  (status) => {
    if (status === 401) void router.replace('/signin')
  },
  { immediate: true },
)

// The admin pages are for Admins whose session proved two-factor (docs/adr/0005).
watch(
  me,
  (p) => {
    if (!props.admin || !p) return
    if (!p.admin) void router.replace('/')
    else if (!p.twoFactor) void router.replace({ path: '/security/two-factor', query: { for: 'admin' } })
  },
  { immediate: true },
)

const personal = [
  { to: '/', label: 'Apps' },
  { to: '/account', label: 'Account' },
  { to: '/security', label: 'Security' },
  { to: '/tokens', label: 'Tokens' },
]
const adminTabs = computed(() => [
  { to: '/admin', label: 'Dashboard' },
  { to: '/admin/unassigned', label: 'Unassigned', count: waiting.value },
  { to: '/admin/groups', label: 'Groups' },
  { to: '/admin/apps', label: 'Apps' },
  { to: '/admin/people', label: 'People' },
  { to: '/admin/machines', label: 'Machines' },
  { to: '/admin/audit', label: 'Audit' },
])
const tabs = computed(() => (props.admin ? adminTabs.value : personal))
function current(to: string): boolean {
  return to === '/' || to === '/admin' ? route.path === to : route.path === to || route.path.startsWith(`${to}/`)
}

const signOut = useAction(signOutMutation())
function leave() {
  signOut.run({}, () => { navigation.leave('/'); })
}
</script>

<template>
  <header class="bar">
    <div class="bar-in">
      <RouterLink class="brand" to="/"><span class="mark"><Icon name="mark" /></span>identity</RouterLink>
      <div v-if="me?.admin" class="viewsw" role="group" aria-label="View">
        <RouterLink to="/" :aria-current="admin ? undefined : 'true'">Personal</RouterLink>
        <RouterLink to="/admin" :aria-current="admin ? 'true' : undefined">Admin<span v-if="waiting > 0" class="dot" :aria-label="`${waiting} waiting`" /></RouterLink>
      </div>
      <nav class="tabs" :aria-label="admin ? 'Admin' : 'Main'">
        <RouterLink v-for="t in tabs" :key="t.to" class="tab" :to="t.to" :aria-current="current(t.to) ? 'page' : undefined">
          {{ t.label }}<span v-if="'count' in t && t.count" class="count">{{ t.count }}</span>
        </RouterLink>
      </nav>
      <div class="bar-end">
        <details class="menu nav-menu">
          <summary class="icon-btn" aria-label="Menu"><Icon name="menu" /></summary>
          <nav class="sheet" aria-label="Main">
            <div class="sheet-label">Personal</div>
            <RouterLink v-for="t in personal" :key="t.to" :to="t.to" :aria-current="current(t.to) ? 'page' : undefined">{{ t.label }}</RouterLink>
            <template v-if="me?.admin">
              <div class="sheet-label">Admin</div>
              <RouterLink v-for="t in adminTabs" :key="t.to" :to="t.to" :aria-current="current(t.to) ? 'page' : undefined">
                {{ t.label }}<span v-if="t.count" class="count">{{ t.count }}</span>
              </RouterLink>
            </template>
          </nav>
        </details>
        <details v-if="me" class="menu">
          <summary class="me-btn" :aria-label="me.name"><PersonAvatar :name="me.name" /><Icon name="chev" /></summary>
          <div class="sheet">
            <div class="sheet-who"><PersonAvatar :name="me.name" size="lg" /><div><b>{{ me.name }}</b><small>{{ me.email }}</small></div></div>
            <RouterLink to="/account">Account</RouterLink><RouterLink to="/security">Security</RouterLink><RouterLink to="/tokens">Tokens</RouterLink>
            <hr />
            <button class="row" type="button" :disabled="signOut.isPending.value" @click="leave"><Icon name="logout" />Sign out</button>
          </div>
        </details>
      </div>
    </div>
  </header>
  <main :class="['wrap', { 'narrow-page': narrow }]">
    <slot />
  </main>
</template>
