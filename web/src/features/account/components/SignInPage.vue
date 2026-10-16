<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { completeSignInMutation, signInMutation } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { navigation } from '../../../infrastructure/navigation'
import { useAction } from '../../../infrastructure/useAction'

const route = useRoute()
const router = useRouter()
const email = ref('')
const password = ref('')
const code = ref('')
const askingForCode = ref(false)
const signIn = useAction(signInMutation())
const complete = useAction(completeSignInMutation())

// Set when an app sent the person here to sign in; once signed in, the Provider finishes for it.
const authRequestID = typeof route.query.authRequestID === 'string' ? route.query.authRequestID : ''

function signedIn() {
  if (authRequestID) navigation.leave(`/oidc/login?authRequestID=${encodeURIComponent(authRequestID)}`)
  else void router.push('/profile')
}

function submit() {
  signIn.run({ body: { email: email.value, password: password.value } }, (answer) => {
    if (answer?.secondFactor) askingForCode.value = true
    else signedIn()
  })
}

function submitCode() {
  complete.run({ body: { code: code.value.replace(/\s/g, '') } }, signedIn)
}
</script>

<template>
  <section aria-labelledby="signin-title">
    <template v-if="askingForCode">
      <h1 id="signin-title">Enter your code</h1>
      <form @submit.prevent="submitCode">
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
