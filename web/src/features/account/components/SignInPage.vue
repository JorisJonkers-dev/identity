<script setup lang="ts">
import { computed, ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import {
  completeSignInMutation,
  completeSignInWithPasskeyMutation,
  passkeyOptionsMutation,
  signInMutation,
} from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { SecondFactorKind } from '../../../infrastructure/api/types.gen'
import { navigation } from '../../../infrastructure/navigation'
import { passkeys } from '../../../infrastructure/passkeys'
import { useAction } from '../../../infrastructure/useAction'

const route = useRoute()
const router = useRouter()
const email = ref('')
const password = ref('')
const code = ref('')
const methods = ref<SecondFactorKind[]>([])
const signIn = useAction(signInMutation())
const complete = useAction(completeSignInMutation())
const keyOptions = useAction(passkeyOptionsMutation())
const keyAnswer = useAction(completeSignInWithPasskeyMutation())
const browserRefused = ref<string | null>(null)
const keyError = computed(() => browserRefused.value ?? keyOptions.error.value ?? keyAnswer.error.value)

// Set when an app sent the person here to sign in; once signed in, the Provider finishes for it.
const authRequestID = typeof route.query.authRequestID === 'string' ? route.query.authRequestID : ''
// Set when the Gate sent the person here for a gated app; only the Gate's own start path is kept.
const gate = typeof route.query.gate === 'string' && route.query.gate.startsWith('/gate/start?') ? route.query.gate : ''

function signedIn() {
  if (authRequestID) navigation.leave(`/oidc/login?authRequestID=${encodeURIComponent(authRequestID)}`)
  else if (gate) navigation.leave(gate)
  else void router.push('/')
}

function submit() {
  signIn.run({ body: { email: email.value, password: password.value } }, (answer) => {
    if (answer?.methods) methods.value = answer.methods
    else signedIn()
  })
}

function usePasskey() {
  browserRefused.value = null
  keyOptions.run({}, (o) => {
    passkeys.get(o.options).then(
      (response) => {
        keyAnswer.run({ body: { response } }, signedIn)
      },
      () => (browserRefused.value = 'Passkey sign-in did not finish. Try again, or enter a 6-digit code from your authenticator app.'),
    )
  })
}

function submitCode() {
  complete.run({ body: { code: code.value.replace(/\s/g, '') } }, signedIn)
}
</script>

<template>
  <section aria-labelledby="signin-title">
    <template v-if="methods.length > 0">
      <h1 id="signin-title">Confirm it's you</h1>
      <template v-if="methods.includes('passkey')">
        <p>Click Use your passkey. Your browser then asks you to confirm it's you.</p>
        <p v-if="keyError" role="alert" class="error">{{ keyError }}</p>
        <button type="button" :disabled="keyOptions.isPending.value || keyAnswer.isPending.value" @click="usePasskey">Use your passkey</button>
      </template>
      <form v-if="methods.includes('totp')" @submit.prevent="submitCode">
        <p>Open your authenticator app and enter the 6-digit code it shows for jorisjonkers.dev. This sign-in expires after 5 minutes.</p>
        <label for="signin-code">6-digit code</label>
        <input id="signin-code" v-model="code" inputmode="numeric" autocomplete="one-time-code" maxlength="8" required />
        <p v-if="complete.error.value" role="alert" class="error">{{ complete.error.value }}</p>
        <button type="submit" :disabled="complete.isPending.value">Sign in</button>
      </form>
    </template>
    <template v-else>
      <h1 id="signin-title">Sign in</h1>
      <form @submit.prevent="submit">
        <label for="signin-email">Email</label>
        <input id="signin-email" v-model="email" type="email" autocomplete="username" required />
        <label for="signin-password">Password</label>
        <input id="signin-password" v-model="password" type="password" autocomplete="current-password" required />
        <p v-if="signIn.error.value" role="alert" class="error">{{ signIn.error.value }}</p>
        <button type="submit" :disabled="signIn.isPending.value">Sign in</button>
      </form>
      <p>No account yet? <RouterLink to="/signup">Sign up</RouterLink></p>
    </template>
  </section>
</template>
