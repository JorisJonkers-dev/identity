<script setup lang="ts">
import { computed, ref } from 'vue'
import type { PersonalToken, TokenClient, TokenDraft } from '../../../infrastructure/api/types.gen'
import AppIcon from '../../../shared/AppIcon.vue'
import AppShell from '../../../shared/AppShell.vue'
import GlassSelect from '../../../shared/GlassSelect.vue'
import Icon from '../../../shared/Icon'
import DialogScrim from '../../../shared/DialogScrim.vue'
import { useCopy } from '../../../shared/useCopy'
import { useTokens } from '../composables/useTokens'
import { expiry } from '../expiry'

const { tokens, clients, loadError, actionError, secret, mint, forgetSecret, revoke } = useTokens()
const now = new Date()
const making = ref(false)
const name = ref('')
const client = ref('')
const days = ref<TokenDraft['days']>(90)
const revoking = ref<string | null>(null)
const { copied, copy } = useCopy()
const clientOptions = computed(() => clients.value.map((c) => ({ value: c.id, label: c.label })))
const live = (t: PersonalToken) => t.state === 'active' || t.state === 'expiring'
const word: Record<PersonalToken['state'], string> = { active: 'Active', expiring: 'Expiring', expired: 'Expired', revoked: 'Revoked' }
const tone: Record<PersonalToken['state'], string> = { active: 'ok', expiring: 'warn', expired: 'off', revoked: 'off' }

function open() {
  name.value = ''
  // The button waits for an app, so there is a first one.
  client.value = (clients.value[0] as TokenClient).id
  days.value = 90
  making.value = true
}
function create() {
  mint({ name: name.value, clientId: client.value, days: days.value })
  making.value = false
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
        <span class="roles"><span v-for="r in t.roles" :key="r" class="role">{{ r }}</span><span v-if="t.roles.length === 0" class="muted">No role</span></span>
        <span class="muted">{{ expiry(t, now) }}</span>
        <span :class="['state', tone[t.state]]">{{ word[t.state] }}</span>
        <span class="act"><button v-if="live(t) && revoking !== t.id" class="btn ghost sm" type="button" :aria-label="`Revoke ${t.name}`" @click="revoking = t.id">Revoke</button></span>
        <div v-if="revoking === t.id" class="lt-confirm">
          <span>Revoke {{ t.name }}? Scripts using it stop working.</span>
          <button class="btn sm" type="button" @click="revoking = null">Cancel</button>
          <button class="btn sm danger-solid" type="button" @click="revoke(t.id); revoking = null">Revoke</button>
        </div>
      </div>
    </div>
    <div v-else-if="tokens" class="glass empty">
      <p>No tokens yet.</p>
      <p v-if="clients.length === 0" class="soft">A token opens one app you have a role in. You have none yet.</p>
    </div>

    <DialogScrim v-if="making" @close="making = false">
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
    </DialogScrim>
    <DialogScrim v-if="secret" @close="forgetSecret">
      <div class="dialog glass strong" role="dialog" aria-modal="true" aria-labelledby="sc-h">
        <h2 id="sc-h">Your new token</h2>
        <p class="warn-line"><Icon name="alert" />Shown once.</p>
        <div class="secret"><code>{{ secret }}</code><button class="btn sm primary" type="button" @click="copy(secret)"><Icon name="copy" />{{ copied ? 'Copied' : 'Copy' }}</button></div>
        <div class="btn-row end"><button class="btn" type="button" @click="forgetSecret">Done</button></div>
      </div>
    </DialogScrim>
  </AppShell>
</template>
