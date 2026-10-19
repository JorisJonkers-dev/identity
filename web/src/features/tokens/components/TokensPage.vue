<script setup lang="ts">
import { computed, ref } from 'vue'
import type { PersonalToken, TokenDraft } from '../../../infrastructure/api/types.gen'
import AppIcon from '../../../shared/AppIcon.vue'
import AppShell from '../../../shared/AppShell.vue'
import GlassSelect from '../../../shared/GlassSelect.vue'
import Icon from '../../../shared/Icon'
import { useTokens } from '../composables/useTokens'
import { expiry } from '../expiry'

const { tokens, clients, loadError, actionError, secret, mint, forgetSecret, revoke } = useTokens()
const now = new Date()
const making = ref(false)
const name = ref('')
const client = ref('')
const days = ref<TokenDraft['days']>(90)
const revoking = ref<string | null>(null)
const copied = ref(false)
const clientOptions = computed(() => clients.value.map((c) => ({ value: c.id, label: c.label })))
const live = (t: PersonalToken) => t.state === 'active' || t.state === 'expiring'
const word: Record<PersonalToken['state'], string> = { active: 'Active', expiring: 'Expiring', expired: 'Expired', revoked: 'Revoked' }
const tone: Record<PersonalToken['state'], string> = { active: 'ok', expiring: 'warn', expired: 'off', revoked: 'off' }

function open() {
  name.value = ''
  client.value = clients.value[0]?.id ?? ''
  days.value = 90
  making.value = true
}
function create() {
  mint({ name: name.value, clientId: client.value, days: days.value })
  making.value = false
}
function copy() {
  if (!secret.value) return
  void navigator.clipboard.writeText(secret.value).then(() => {
    copied.value = true
    setTimeout(() => (copied.value = false), 1400)
  })
}
</script>

<template>
  <AppShell>
    <header class="head">
      <h1>Tokens</h1>
      <div class="end"><button class="btn primary" type="button" :disabled="clients.length === 0" @click="open"><Icon name="plus" />New token</button></div>
    </header>
    <p v-if="loadError || actionError" role="alert" class="alert"><Icon name="alert" />{{ loadError ?? actionError }}</p>
    <div v-if="tokens && tokens.length" class="glass lt">
      <div class="lt-h cols-tokens"><span>Name</span><span>App</span><span>Roles</span><span>Expires</span><span>State</span><span /></div>
      <div v-for="t in tokens" :key="t.id" :class="['lt-r cols-tokens', { off: !live(t) }]">
        <span class="name">{{ t.name }}</span>
        <span class="app-c"><AppIcon :label="t.client.label" size="xs" />{{ t.client.label }}</span>
        <span class="roles"><span v-for="r in t.roles" :key="r" class="role">{{ r }}</span></span>
        <span class="muted">{{ expiry(t, now) }}</span>
        <span :class="['state', tone[t.state]]">{{ word[t.state] }}</span>
        <span class="act"><button v-if="live(t) && revoking !== t.id" class="btn ghost sm" type="button" @click="revoking = t.id">Revoke</button></span>
        <div v-if="revoking === t.id" class="lt-confirm">
          <span>Revoke {{ t.name }}?</span>
          <button class="btn sm" type="button" @click="revoking = null">Cancel</button>
          <button class="btn sm danger-solid" type="button" @click="revoke(t.id); revoking = null">Revoke</button>
        </div>
      </div>
    </div>
    <div v-else-if="tokens" class="glass empty"><p>No tokens yet.</p></div>

    <div v-if="making" class="scrim" @click.self="making = false">
      <form class="dialog glass strong" role="dialog" aria-modal="true" aria-labelledby="nt-h" @submit.prevent="create">
        <h2 id="nt-h">New token</h2>
        <div class="field"><label class="label" for="nt-name">Name</label><input id="nt-name" v-model="name" class="input md mono" required maxlength="64" /></div>
        <div class="field"><span class="label">App</span><GlassSelect v-model="client" :options="clientOptions" label="App" /></div>
        <div class="field">
          <span class="label">Expires after</span>
          <div class="seg fill" role="group" aria-label="Expires after">
            <button v-for="d in [30, 90, 365] as const" :key="d" type="button" :aria-pressed="days === d" @click="days = d">{{ d }} days</button>
          </div>
        </div>
        <div class="btn-row end"><button class="btn" type="button" @click="making = false">Cancel</button><button class="btn primary" type="submit" :disabled="!name || !client">Create token</button></div>
      </form>
    </div>
    <div v-if="secret" class="scrim">
      <div class="dialog glass strong" role="dialog" aria-modal="true" aria-labelledby="sc-h">
        <h2 id="sc-h">Your new token</h2>
        <p class="warn-line"><Icon name="alert" />Shown once.</p>
        <div class="secret"><code>{{ secret }}</code><button class="btn sm primary" type="button" @click="copy"><Icon name="copy" />{{ copied ? 'Copied' : 'Copy' }}</button></div>
        <div class="btn-row end"><button class="btn" type="button" @click="forgetSecret">Done</button></div>
      </div>
    </div>
  </AppShell>
</template>
