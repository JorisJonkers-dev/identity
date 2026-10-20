<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import {
  completeSignInMutation,
  completeSignInWithPasskeyMutation,
  passkeyOptionsMutation,
  signInMutation,
} from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { TwoFactorMethodKind } from '../../../infrastructure/api/types.gen'
import { navigation } from '../../../infrastructure/navigation'
import { passkeys } from '../../../infrastructure/passkeys'
import { useAction } from '../../../infrastructure/useAction'
import AuthShell from '../../../shared/AuthShell.vue'
import CodeBoxes from '../../../shared/CodeBoxes.vue'
import Icon from '../../../shared/Icon'

const route = useRoute()
const router = useRouter()
const email = ref('')
const password = ref('')
const shown = ref(false)
const code = ref('')
const methods = ref<TwoFactorMethodKind[]>([])
const signIn = useAction(signInMutation())
const complete = useAction(completeSignInMutation())
const keyOptions = useAction(passkeyOptionsMutation())
const keyAnswer = useAction(completeSignInWithPasskeyMutation())
const browserRefused = ref<string | null>(null)
const keyError = computed(() => browserRefused.value ?? keyOptions.error.value ?? keyAnswer.error.value)
// The first field takes the cursor, and so does the code once the password is right.
const emailField = ref<HTMLInputElement | null>(null)
const codeBoxes = ref<{ focus: () => void } | null>(null)
onMounted(() => emailField.value?.focus())
watch(methods, () => {
  void nextTick(() => codeBoxes.value?.focus())
})
const keyBusy = computed(() => keyOptions.isPending.value || keyAnswer.isPending.value)

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
      (response) => { keyAnswer.run({ body: { response } }, signedIn); },
      () => (browserRefused.value = 'Passkey sign-in did not finish. Try again.'),
    )
  })
}
function submitCode() {
  complete.run({ body: { code: code.value } }, signedIn)
}
</script>

<template>
  <AuthShell>
    <form v-if="methods.length > 0" class="auth-box" novalidate @submit.prevent="submitCode">
      <span class="from who"><span>{{ email }}</span></span>
      <h1>{{ methods.includes('totp') ? 'Enter your code' : 'Use your passkey' }}</h1>
      <template v-if="methods.includes('totp')">
        <CodeBoxes ref="codeBoxes" v-model="code" :invalid="!!complete.error.value" />
        <p v-if="complete.error.value" role="alert" class="alert"><Icon name="alert" />{{ complete.error.value }}</p>
        <button class="btn primary lg wide" type="submit" :disabled="complete.isPending.value || code.length < 6">Verify</button>
      </template>
      <template v-if="methods.includes('passkey')">
        <div v-if="methods.includes('totp')" class="or">or</div>
        <p v-if="keyError" role="alert" class="alert"><Icon name="alert" />{{ keyError }}</p>
        <button class="btn lg wide" type="button" :disabled="keyBusy" @click="usePasskey"><Icon name="passkey" />Use your passkey</button>
      </template>
    </form>
    <form v-else class="auth-box" novalidate @submit.prevent="submit">
      <h1>Sign in</h1>
      <div class="field"><label class="label" for="si-email">Email</label><input id="si-email" ref="emailField" v-model="email" class="input" type="email" autocomplete="username webauthn" required /></div>
      <div class="field">
        <div class="label"><label for="si-pw">Password</label><RouterLink class="link" to="/forgot">Forgot password?</RouterLink></div>
        <div class="pw">
          <input id="si-pw" v-model="password" class="input" :type="shown ? 'text' : 'password'" autocomplete="current-password" required :aria-invalid="!!signIn.error.value || undefined" />
          <button type="button" :aria-label="shown ? 'Hide password' : 'Show password'" @click="shown = !shown"><Icon name="eye" /></button>
        </div>
      </div>
      <p v-if="signIn.error.value" role="alert" class="alert"><Icon name="alert" />{{ signIn.error.value }}</p>
      <button class="btn primary lg wide" type="submit" :disabled="signIn.isPending.value" :aria-busy="signIn.isPending.value">
        <span v-if="signIn.isPending.value" class="spin" aria-hidden="true" />{{ signIn.isPending.value ? 'Signing in' : 'Sign in' }}
      </button>
      <p class="auth-foot">No account? <RouterLink class="link" to="/signup">Sign up</RouterLink></p>
    </form>
  </AuthShell>
</template>
