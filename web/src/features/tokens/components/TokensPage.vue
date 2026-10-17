<script setup lang="ts">
import { reactive, ref } from 'vue'
import type { TokenDraft } from '../../../infrastructure/api/types.gen'
import YourLayout from '../../../shared/YourLayout.vue'
import { useTokens } from '../composables/useTokens'
import { expiry } from '../expiry'

const { tokens, clients, isLoading, loadError, actionError, secret, mint, forgetSecret, revoke } = useTokens()
const now = new Date()
const making = ref(false)
const draft = reactive<TokenDraft>({ name: '', clientId: '', days: 90 })
const asking = ref<string | null>(null)
const copied = ref(false)

function create() {
  mint({ ...draft })
}

function done() {
  forgetSecret()
  making.value = false
  copied.value = false
  draft.name = ''
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    copied.value = true
  } catch {
    copied.value = false
  }
}

function confirmRevoke(id: string) {
  asking.value = null
  revoke(id)
}

const pill = 'pill gap-[5px]'
const tone = { active: 'border-transparent bg-ok-soft text-ok', expiring: 'border-transparent bg-warn-soft text-warn', expired: '', revoked: '' }
const field = 'w-full rounded-[9px] border border-line bg-surface px-3 py-2.5 font-body text-sm text-ink'
const label = 'grid gap-1.5 text-[12.5px] font-medium text-ink-2'
const primary = 'btn border-accent bg-accent text-accent-ink'
</script>

<template>
  <YourLayout>
    <div class="flex flex-wrap items-end justify-between gap-3">
      <div>
        <h1 class="m-0 font-display text-[22px]/[1.15] font-bold tracking-[-0.02em] text-balance">Tokens</h1>
        <p class="mt-0.5 mb-0 text-[13px] text-ink-3">For scripts and tools. A token never has more access than you do.</p>
      </div>
      <button v-if="!making && clients.length > 0" type="button" :class="primary" @click="making = true">New token</button>
    </div>
    <p v-if="actionError" role="alert" class="m-0 text-bad">{{ actionError }}</p>

    <section v-if="secret" class="panel grid gap-3" aria-labelledby="secret-title" role="status">
      <h2 id="secret-title" class="m-0 text-[13px] font-semibold">Copy your token now. You won't see it again.</h2>
      <code class="block overflow-x-auto rounded-[9px] border border-line bg-surface-2 px-3 py-2.5 font-mono text-xs break-all" data-secret>{{ secret }}</code>
      <div class="flex flex-wrap gap-2">
        <button type="button" class="btn" @click="copy(secret)">{{ copied ? 'Copied' : 'Copy' }}</button>
        <button type="button" :class="primary" @click="done">Done</button>
      </div>
    </section>

    <form v-else-if="making" class="panel grid gap-3.5" aria-label="New token" @submit.prevent="create">
      <label :class="label">Name<input v-model="draft.name" :class="field" required maxlength="64" placeholder="backup-script" /></label>
      <label :class="label">
        App
        <select v-model="draft.clientId" :class="field" required>
          <option value="" disabled>Choose an app</option>
          <option v-for="c in clients" :key="c.id" :value="c.id">{{ c.label }}</option>
        </select>
      </label>
      <label :class="label">
        Lasts
        <select v-model.number="draft.days" :class="field">
          <option :value="30">30 days</option>
          <option :value="90">90 days</option>
          <option :value="365">365 days</option>
        </select>
      </label>
      <div class="flex flex-wrap gap-2">
        <button type="submit" :class="primary">Create token</button>
        <button type="button" class="btn" @click="making = false">Cancel</button>
      </div>
    </form>

    <section class="panel" aria-labelledby="tokens-title">
      <h2 id="tokens-title" class="mt-0 mb-2.5 text-[13px] font-semibold">Personal Tokens</h2>
      <p v-if="isLoading" class="m-0 text-ink-3">Loading…</p>
      <p v-else-if="loadError" role="alert" class="m-0 text-bad">{{ loadError }}</p>
      <p v-else-if="tokens?.length === 0" class="m-0 text-ink-3">
        You have no tokens.<template v-if="clients.length === 0"> A token is for an app you have a role in, and you have none yet.</template>
      </p>
      <ul v-else class="m-0 grid list-none p-0" aria-labelledby="tokens-title">
        <li
          v-for="t in tokens"
          :key="t.id"
          class="flex min-w-0 flex-wrap items-center gap-3 border-t border-line py-2.5 first:border-t-0 first:pt-0"
          :aria-label="t.name"
        >
          <div class="min-w-0 flex-1">
            <b>{{ t.name }}</b>
            <small class="block truncate text-xs text-ink-3">
              {{ t.client.label }} · <span class="font-mono">{{ t.roles.join(', ') || 'no role' }}</span> · {{ expiry(t, now) }}
            </small>
          </div>
          <span :class="[pill, tone[t.state]]"><i class="size-1.5 rounded-full bg-current"></i>{{ t.state }}</span>
          <template v-if="t.state === 'active' || t.state === 'expiring'">
            <template v-if="asking === t.id">
              <span class="text-[13px] font-medium">Revoke {{ t.name }}? Scripts using it stop working.</span>
              <button type="button" class="btn text-bad" @click="confirmRevoke(t.id)">Yes, revoke</button>
              <button type="button" class="btn" @click="asking = null">Cancel</button>
            </template>
            <button v-else type="button" class="btn text-bad" :aria-label="`Revoke ${t.name}`" @click="asking = t.id">Revoke</button>
          </template>
        </li>
      </ul>
    </section>
  </YourLayout>
</template>
