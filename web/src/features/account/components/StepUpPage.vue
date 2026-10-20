<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import {
  startStepUpMutation,
  stepUpPasskeyOptionsMutation,
  stepUpWithCodeMutation,
  stepUpWithPasskeyMutation,
} from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { TwoFactorMethodKind } from '../../../infrastructure/api/types.gen'
import { navigation } from '../../../infrastructure/navigation'
import { passkeys } from '../../../infrastructure/passkeys'
import { useAction } from '../../../infrastructure/useAction'
import AuthShell from '../../../shared/AuthShell.vue'
import CodeBoxes from '../../../shared/CodeBoxes.vue'
import Icon from '../../../shared/Icon'

/** An app wants a fresher two-factor proof (step-up): the signed-in User proves it, then goes back. */
const route = useRoute()
const router = useRouter()
const authRequestID = typeof route.query.authRequestID === 'string' ? route.query.authRequestID : ''
// A page of ours that sent the person here, to go back to; anything else goes to the apps.
const next = typeof route.query.next === 'string' && /^\/[^/\\]/.test(route.query.next) ? route.query.next : '/'
const code = ref('')
const methods = ref<TwoFactorMethodKind[]>([])
const start = useAction(startStepUpMutation())
const complete = useAction(stepUpWithCodeMutation())
const keyOptions = useAction(stepUpPasskeyOptionsMutation())
const keyAnswer = useAction(stepUpWithPasskeyMutation())
const browserRefused = ref<string | null>(null)
const keyError = computed(() => browserRefused.value ?? keyOptions.error.value ?? keyAnswer.error.value)
const keyBusy = computed(() => keyOptions.isPending.value || keyAnswer.isPending.value)
const codeBoxes = ref<{ focus: () => void } | null>(null)
watch(methods, () => {
  void nextTick(() => codeBoxes.value?.focus())
})
watch(start.status, (status) => {
  if (status === 401) void router.replace({ path: '/signin', query: authRequestID ? { authRequestID } : {} })
})
onMounted(() => {
  start.run({}, (answer) => (methods.value = answer.methods))
})

function proved() {
  if (authRequestID) navigation.leave(`/oidc/login?authRequestID=${encodeURIComponent(authRequestID)}`)
  // The Gate's start is served by the server, not this app.
  else if (next.startsWith('/gate/')) navigation.leave(next)
  else void router.push(next)
}
function submitCode() {
  complete.run({ body: { code: code.value } }, proved)
}
function usePasskey() {
  browserRefused.value = null
  keyOptions.run({}, (o) => {
    passkeys.get(o.options).then(
      (response) => { keyAnswer.run({ body: { response } }, proved); },
      () => (browserRefused.value = 'Passkey check was cancelled. Try again.'),
    )
  })
}
</script>

<template>
  <AuthShell>
    <div v-if="start.error.value && start.status.value !== 401" class="auth-box">
      <h1>Two-factor needed</h1>
      <p role="alert" class="alert"><Icon name="alert" />{{ start.error.value }}</p>
      <RouterLink class="btn primary lg wide" to="/security/two-factor?for=step-up">Set up two-factor</RouterLink>
    </div>
    <form v-else-if="methods.length > 0" class="auth-box" novalidate @submit.prevent="submitCode">
      <h1>{{ methods.includes('totp') ? 'Enter your code' : 'Use your passkey' }}</h1>
      <p class="lead">This app asks for two-factor again before it opens.</p>
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
  </AuthShell>
</template>
