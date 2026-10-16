<script setup lang="ts">
import { reactive, ref } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useSecurity } from '../composables/useSecurity'
import QrCode from './QrCode.vue'

const route = useRoute()
const { factors, isLoading, loadError, setup, start, confirm, remove } = useSecurity()
const label = ref('')
const code = ref('')
const password = ref('')
const added = ref(false)
const removing = ref<string | null>(null)
const removalPasswords = reactive<Record<string, string>>({})

// The access page sends an Admin here when their session proved no Second Factor.
const forAdmin = route.query.for === 'admin'

function dateOf(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })
}

function submitConfirm() {
  confirm.run(label.value, code.value, password.value, () => {
    label.value = code.value = password.value = ''
    added.value = true
  })
}

function submitRemoval(id: string) {
  remove.run(id, removalPasswords[id] ?? '')
}
</script>

<template>
  <section aria-labelledby="security-title">
    <h1 id="security-title">Security</h1>
    <p v-if="forAdmin && factors.length > 0 && !isLoading" role="status" class="notice">
      Admins can manage access only from a session signed in with an authenticator app. Sign out, then sign in again and enter a code from your app.
    </p>
    <p v-else-if="forAdmin && !isLoading" role="status" class="notice">
      Admins need an authenticator app to manage access. Add one below, then go back to <RouterLink to="/access">Access</RouterLink>.
    </p>
    <p>An authenticator app gives you a 6-digit code to enter after your password. Your password alone cannot sign you in.</p>

    <p v-if="isLoading">Loading…</p>
    <p v-else-if="loadError" role="alert" class="error">{{ loadError }} <RouterLink to="/signin">Sign in</RouterLink></p>
    <template v-else>
      <h2 id="factors-title">Your authenticator apps</h2>
      <p v-if="factors.length === 0">You have no authenticator apps yet.</p>
      <ul v-else aria-labelledby="factors-title">
        <li v-for="f in factors" :key="f.id">
          <b>{{ f.label }}</b>
          <span>Added {{ dateOf(f.createdAt) }}</span>
          <span v-if="f.lastUsedAt"> · last used {{ dateOf(f.lastUsedAt) }}</span>
          <form v-if="removing === f.id" class="inline" @submit.prevent="submitRemoval(f.id)">
            <label :for="`remove-${f.id}`">Your password</label>
            <input :id="`remove-${f.id}`" v-model="removalPasswords[f.id]" type="password" autocomplete="current-password" required />
            <button type="submit" :disabled="remove.isPending.value">Remove {{ f.label }}</button>
            <button type="button" @click="removing = null">Cancel</button>
          </form>
          <button v-else type="button" @click="removing = f.id">Remove app</button>
        </li>
      </ul>
      <p v-if="remove.error.value" role="alert" class="error">{{ remove.error.value }}</p>
      <p v-if="added" role="status">Authenticator app added. You were signed out on every other device. Next time you sign in, enter a code from it.</p>

      <h2>Add an authenticator app</h2>
      <button v-if="!setup" type="button" :disabled="start.isPending.value" @click="start.run()">Add an authenticator app</button>
      <p v-if="start.error.value" role="alert" class="error">{{ start.error.value }}</p>
      <form v-if="setup" @submit.prevent="submitConfirm">
        <p>Scan this QR code with your authenticator app, or type the key into it.</p>
        <QrCode :value="setup.uri" label="QR code for your authenticator app" />
        <p>Key: <code class="secret">{{ setup.secret }}</code></p>
        <label for="totp-label">Name for this app</label>
        <input id="totp-label" v-model="label" maxlength="64" placeholder="Phone" required />
        <label for="totp-code">6-digit code from your app</label>
        <input id="totp-code" v-model="code" inputmode="numeric" autocomplete="one-time-code" maxlength="8" required />
        <label for="totp-password">Your password</label>
        <input id="totp-password" v-model="password" type="password" autocomplete="current-password" required />
        <p v-if="confirm.error.value" role="alert" class="error">{{ confirm.error.value }}</p>
        <button type="submit" :disabled="confirm.isPending.value">Add authenticator app</button>
      </form>
    </template>
  </section>
</template>

<style scoped>
.error {
  color: #a4161a;
}
.notice {
  padding: 0.5rem 0.75rem;
  border: 1px solid #c8c8c8;
}
.secret {
  overflow-wrap: anywhere;
}
.inline {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  align-items: center;
}
.inline > * {
  min-width: 0;
  max-width: 100%;
}
</style>
