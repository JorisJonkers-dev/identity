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
import AuthLayout from '../../../shared/AuthLayout.vue'
import CardLayout from '../../../shared/CardLayout.vue'

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
  <CardLayout v-if="methods.length > 0" glyph="lock">
    <section aria-labelledby="signin-title" class="grid gap-3.5">
      <h1 id="signin-title" class="page-title">Confirm it's you</h1>
      <div v-if="methods.includes('passkey')" class="grid gap-3">
        <p class="fine">Your browser asks you to confirm with your passkey.</p>
        <p v-if="keyError" role="alert" class="alert">{{ keyError }}</p>
        <button
          type="button"
          :class="[methods.includes('totp') ? 'btn' : 'btn-primary', 'w-full py-[11px]']"
          :disabled="keyOptions.isPending.value || keyAnswer.isPending.value"
          @click="usePasskey"
        >
          Use your passkey
        </button>
      </div>
      <p v-if="methods.includes('passkey') && methods.includes('totp')" class="flex items-center gap-2.5 text-[12px] text-ink-3 before:h-px before:flex-1 before:bg-line after:h-px after:flex-1 after:bg-line">or</p>
      <form v-if="methods.includes('totp')" class="grid gap-3" @submit.prevent="submitCode">
        <p class="fine">Enter the code your authenticator app shows.</p>
        <label for="signin-code" class="field-label">6-digit code</label>
        <input
          id="signin-code"
          v-model="code"
          class="input py-3 text-center font-mono text-[22px] font-semibold tracking-[0.5em]"
          inputmode="numeric"
          autocomplete="one-time-code"
          maxlength="8"
          required
        />
        <p v-if="complete.error.value" role="alert" class="alert">{{ complete.error.value }}</p>
        <button type="submit" class="btn-primary w-full py-[11px]" :disabled="complete.isPending.value">Sign in</button>
      </form>
    </section>
  </CardLayout>
  <AuthLayout v-else>
    <section aria-labelledby="signin-title" class="grid gap-3.5">
      <h1 id="signin-title" class="page-title">Sign in</h1>
      <form class="grid gap-3.5" @submit.prevent="submit">
        <div class="grid gap-1.5">
          <label for="signin-email" class="field-label">Email</label>
          <input id="signin-email" v-model="email" class="input" type="email" autocomplete="username" required />
        </div>
        <div class="grid gap-1.5">
          <label for="signin-password" class="field-label">Password</label>
          <input id="signin-password" v-model="password" class="input" type="password" autocomplete="current-password" required />
        </div>
        <p v-if="signIn.error.value" role="alert" class="alert">{{ signIn.error.value }}</p>
        <button type="submit" class="btn-primary w-full py-[11px]" :disabled="signIn.isPending.value">Sign in</button>
      </form>
      <p class="fine">No account yet? <RouterLink to="/signup">Sign up</RouterLink>.</p>
    </section>
  </AuthLayout>
</template>
