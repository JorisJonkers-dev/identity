<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import {
  checkPasswordResetMutation,
  completePasswordResetMutation,
  resetPasskeyOptionsMutation,
  resetWithCodeMutation,
  resetWithPasskeyMutation,
} from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { TwoFactorMethodKind } from '../../../infrastructure/api/types.gen'
import { passkeys } from '../../../infrastructure/passkeys'
import { useAction } from '../../../infrastructure/useAction'
import AuthShell from '../../../shared/AuthShell.vue'
import CodeBoxes from '../../../shared/CodeBoxes.vue'
import Icon from '../../../shared/Icon'
import { adminMail } from '../../../shared/contact'

/** The mailed link to choose a new password: two-factor first when the User has it, then the password. */
const route = useRoute()
const token = typeof route.query.token === 'string' ? route.query.token : ''
const check = useAction(checkPasswordResetMutation())
const withCode = useAction(resetWithCodeMutation())
const keyOptions = useAction(resetPasskeyOptionsMutation())
const keyAnswer = useAction(resetWithPasskeyMutation())
const complete = useAction(completePasswordResetMutation())
const methods = ref<TwoFactorMethodKind[] | null>(null)
const code = ref('')
const password = ref('')
const browserRefused = ref<string | null>(null)
const keyError = computed(() => browserRefused.value ?? keyOptions.error.value ?? keyAnswer.error.value)
const codeBoxes = ref<{ focus: () => void } | null>(null)
const passwordField = ref<HTMLInputElement | null>(null)
watch(methods, () => {
  void nextTick(() => {
    codeBoxes.value?.focus()
    passwordField.value?.focus()
  })
})
onMounted(() => {
  if (token) check.run({ body: { token } }, (answer) => (methods.value = answer.methods))
})
const proved = () => (methods.value = [])
function submitCode() {
  withCode.run({ body: { token, code: code.value } }, proved)
}
function usePasskey() {
  browserRefused.value = null
  keyOptions.run({ body: { token } }, (o) => {
    passkeys.get(o.options).then(
      (response) => { keyAnswer.run({ body: { token, response } }, proved); },
      () => (browserRefused.value = 'Passkey check was cancelled. Try again.'),
    )
  })
}
</script>

<template>
  <AuthShell>
    <div v-if="!token" class="auth-box">
      <span class="big-ico warn"><Icon name="link" /></span>
      <h1>This link is incomplete</h1>
      <p class="lead">Open the full link from the email, or ask for a new one.</p>
      <RouterLink class="btn primary lg wide" to="/forgot">Ask for a new link</RouterLink>
    </div>
    <div v-else-if="check.error.value" class="auth-box">
      <span class="big-ico warn"><Icon name="link" /></span>
      <h1>This link no longer works</h1>
      <p role="alert" class="alert"><Icon name="alert" />{{ check.error.value }}</p>
      <RouterLink class="btn primary lg wide" to="/forgot">Ask for a new link</RouterLink>
    </div>
    <div v-else-if="complete.isDone.value" class="auth-box">
      <span class="big-ico ok"><Icon name="check" /></span>
      <h1>Password changed</h1>
      <p class="lead">Every device was signed out. Sign in with your new password.</p>
      <RouterLink class="btn primary lg wide" to="/signin">Sign in</RouterLink>
    </div>
    <form v-else-if="methods && methods.length > 0" class="auth-box" novalidate @submit.prevent="submitCode">
      <h1>{{ methods.includes('totp') ? 'Enter your code' : 'Use your passkey' }}</h1>
      <p class="lead">Confirm it is you before you choose a new password.</p>
      <template v-if="methods.includes('totp')">
        <CodeBoxes ref="codeBoxes" v-model="code" :invalid="!!withCode.error.value" />
        <p v-if="withCode.error.value" role="alert" class="alert"><Icon name="alert" />{{ withCode.error.value }}</p>
        <button class="btn primary lg wide" type="submit" :disabled="withCode.isPending.value || code.length < 6">Verify</button>
      </template>
      <template v-if="methods.includes('passkey')">
        <div v-if="methods.includes('totp')" class="or">or</div>
        <p v-if="keyError" role="alert" class="alert"><Icon name="alert" />{{ keyError }}</p>
        <button class="btn lg wide" type="button" @click="usePasskey"><Icon name="passkey" />Use your passkey</button>
      </template>
      <p class="auth-foot">Lost your phone or passkey? Contact an Admin at <a class="link" :href="adminMail">{{ adminMail.slice('mailto:'.length) }}</a></p>
    </form>
    <form v-else-if="methods" class="auth-box" novalidate @submit.prevent="complete.run({ body: { token, password } })">
      <h1>Choose a new password</h1>
      <div class="field"><label class="label" for="rs-pw">New password</label><input id="rs-pw" ref="passwordField" v-model="password" class="input" type="password" autocomplete="new-password" required /></div>
      <p v-if="complete.error.value" role="alert" class="alert"><Icon name="alert" />{{ complete.error.value }}</p>
      <button class="btn primary lg wide" type="submit" :disabled="complete.isPending.value">Save password</button>
    </form>
  </AuthShell>
</template>
