<script setup lang="ts">
import { adminMail } from '../../../shared/contact'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import type { LauncherApp } from '../../../infrastructure/api/types.gen'
import AppIcon from '../../../shared/AppIcon.vue'
import AppShell from '../../../shared/AppShell.vue'
import AuthShell from '../../../shared/AuthShell.vue'
import Icon from '../../../shared/Icon'
import { useExpiringTokens } from '../../tokens/composables/useExpiringTokens'
import { expiry } from '../../tokens/expiry'
import { useLauncher } from '../composables/useLauncher'
import VersionLine from './VersionLine.vue'

const { apps, heldBack, isLoading, loadError } = useLauncher()
const ending = useExpiringTokens(computed(() => apps.value !== undefined))
const now = new Date()
const find = ref('')
const search = ref<HTMLInputElement | null>(null)
// An app is found by any part of its name or description.
function matches(app: LauncherApp): boolean {
  const q = find.value.trim().toLowerCase()
  return !q || app.label.toLowerCase().includes(q) || app.description?.toLowerCase().includes(q) === true
}
// "/" puts the cursor in the search, as the design's keycap says.
function slash(e: KeyboardEvent) {
  if (e.key === '/' && !(document.activeElement instanceof HTMLInputElement)) {
    e.preventDefault()
    search.value?.focus()
  }
}
onMounted(() => { document.addEventListener('keydown', slash); })
onBeforeUnmount(() => { document.removeEventListener('keydown', slash); })
</script>

<template>
  <AppShell v-if="apps">
    <div class="launch">
      <h1 class="sr">Apps</h1>
      <label class="search glass strong">
        <Icon name="search" />
        <input ref="search" v-model="find" type="search" placeholder="Find an app" aria-label="Find an app" />
        <span class="kbd" aria-hidden="true">/</span>
      </label>
      <div v-if="ending[0]" class="strip glass" role="status">
        <Icon name="clock" />
        <span v-if="ending.length === 1"><b>{{ ending[0].name }}</b> {{ expiry(ending[0], now) }}.</span>
        <span v-else>{{ ending.length }} tokens expire within 7 days.</span>
        <RouterLink class="btn sm" to="/tokens">View tokens</RouterLink>
      </div>
      <p v-if="heldBack" class="note warn" role="status">
        <Icon name="shield" /><span>Add two-factor to open {{ heldBack }} more {{ heldBack === 1 ? 'app' : 'apps' }}.</span>
        <RouterLink class="link" to="/security/two-factor">Set up two-factor</RouterLink>
      </p>
      <div v-if="apps.length === 0" class="glass apps-empty"><p>No apps yet. An Admin adds you to a Group.</p><a class="link" :href="adminMail">Contact an Admin</a></div>
      <p v-else-if="!apps.some(matches)" class="soft">No app matches “{{ find }}”.</p>
      <div v-else class="apps">
        <a v-for="app in apps.filter(matches)" :key="app.id" class="app" :href="app.launchUrl">
          <AppIcon :label="app.label" :icon-url="app.iconUrl" />
          <div>
            <b>{{ app.label }}</b>
            <div class="roles"><span class="sr">Your roles: </span><span v-for="r in app.roles" :key="r" class="role">{{ r }}</span></div>
          </div>
          <Icon name="out" class="go" />
        </a>
      </div>
      <VersionLine />
    </div>
  </AppShell>
  <AuthShell v-else brandless>
    <div class="auth-box land">
      <span class="land-mark"><Icon name="mark" /></span>
      <h1>jorisjonkers.dev</h1>
      <p v-if="isLoading" class="soft">Loading…</p>
      <p v-else-if="loadError" role="alert" class="alert">{{ loadError }}</p>
      <div v-else class="land-acts">
        <RouterLink class="btn primary xl wide" to="/signin">Sign in</RouterLink>
        <RouterLink class="btn xl wide" to="/signup">Sign up</RouterLink>
      </div>
      <VersionLine />
    </div>
  </AuthShell>
</template>
