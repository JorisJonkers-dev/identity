<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { RouterLink } from 'vue-router'
import type { LauncherApp } from '../../../infrastructure/api/types.gen'
import AuthLayout from '../../../shared/AuthLayout.vue'
import YourLayout from '../../../shared/YourLayout.vue'
import { useExpiringTokens } from '../../tokens/composables/useExpiringTokens'
import { expiry } from '../../tokens/expiry'
import { useLauncher } from '../composables/useLauncher'
import { greeting } from '../greeting'
import VersionLine from './VersionLine.vue'

const { apps, isLoading, loadError, name } = useLauncher()
const ending = useExpiringTokens(computed(() => apps.value !== undefined))
const now = new Date()
// Apps whose icon failed to load show their label's first letter instead.
const broken = reactive(new Set<string>())
const find = ref('')
// An app is found by any part of its name or description.
function matches(app: LauncherApp): boolean {
  const q = find.value.trim().toLowerCase()
  return !q || app.label.toLowerCase().includes(q) || app.description?.toLowerCase().includes(q) === true
}
</script>

<template>
  <YourLayout v-if="apps">
    <section aria-labelledby="home-title" class="grid gap-[18px]">
      <div class="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 id="home-title" class="m-0 font-display text-[22px]/[1.15] font-bold tracking-[-0.02em]">{{ greeting(name, now) }}</h1>
          <p class="m-0 mt-0.5 text-[13px] text-ink-3">{{ apps.length === 1 ? '1 app' : `${apps.length} apps` }}</p>
        </div>
        <label v-if="apps.length > 0" class="flex max-w-[300px] min-w-0 flex-1 items-center gap-2 rounded-[10px] border border-line bg-surface px-3 py-2 text-ink-3">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" /></svg>
          <span class="sr-only">Find an app</span>
          <input v-model="find" type="search" placeholder="Find an app" class="min-w-0 flex-1 border-0 bg-transparent font-body text-[13px] text-ink outline-none" />
        </label>
      </div>
      <p v-if="ending[0]" role="status" class="m-0 rounded-[9px] border border-warn bg-warn-soft px-3 py-2.5 text-[13px] text-warn">
        <template v-if="ending.length === 1">Your token {{ ending[0].name }} {{ expiry(ending[0], now) }}. </template>
        <template v-else>{{ ending.length }} of your tokens expire within 7 days: {{ ending.map((t) => t.name).join(', ') }}. </template>
        <RouterLink to="/profile/tokens" class="text-warn underline">Make a new one on the Tokens page</RouterLink>.
      </p>
      <p v-if="apps.length === 0" class="panel m-0 text-ink-2">You don't have access to any apps yet. Ask an Admin to add you to a Group.</p>
      <p v-else-if="!apps.some(matches)" class="m-0 text-ink-3">No app matches “{{ find }}”.</p>
      <ul v-else class="m-0 grid list-none grid-cols-[repeat(auto-fill,minmax(150px,1fr))] gap-3 p-0" aria-labelledby="home-title">
        <li v-for="app in apps.filter(matches)" :key="app.id" class="app">
          <a
            :href="app.launchUrl"
            class="grid h-full gap-2.5 rounded-[14px] border border-line bg-surface p-3.5 text-inherit no-underline transition hover:-translate-y-0.5 hover:border-ink-3 motion-reduce:transition-none motion-reduce:hover:translate-y-0"
          >
            <img v-if="app.iconUrl && !broken.has(app.id)" :src="app.iconUrl" alt="" width="38" height="38" class="size-[38px] rounded-[10px]" @error="broken.add(app.id)" />
            <span v-else class="letter grid size-[38px] place-items-center rounded-[10px] bg-accent-soft font-display text-[15px] font-bold text-accent" aria-hidden="true">{{
              app.label.charAt(0)
            }}</span>
            <span class="grid">
              <b class="label text-[14px] font-semibold">{{ app.label }}</b>
              <small v-if="app.description" class="description text-[12px] text-ink-3">{{ app.description }}</small>
            </span>
            <span class="roles flex flex-wrap gap-1">
              <span class="sr-only">Your access: </span>
              <span v-for="role in app.roles" :key="role" :class="['pill', role !== 'user' && 'border-transparent bg-accent-soft text-accent']">{{ role }}</span>
            </span>
          </a>
        </li>
      </ul>
      <VersionLine />
    </section>
  </YourLayout>
  <AuthLayout v-else>
    <section aria-labelledby="home-title" class="grid gap-3.5">
      <h1 id="home-title" class="m-0 font-display text-[26px]/[1.1] font-bold tracking-[-0.02em]">jorisjonkers.dev</h1>
      <p class="m-0 text-ink-2">Sign in once to open the apps you have been given access to.</p>
      <p v-if="isLoading" class="fine">Loading…</p>
      <p v-else-if="loadError" role="alert" class="alert">{{ loadError }}</p>
      <div v-else class="grid gap-2.5">
        <RouterLink to="/signin" class="btn-primary py-[11px] text-center">Sign in</RouterLink>
        <RouterLink to="/signup" class="btn py-[11px] text-center">Sign up</RouterLink>
      </div>
      <VersionLine />
    </section>
  </AuthLayout>
</template>
