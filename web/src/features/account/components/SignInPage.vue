<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { signInMutation } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { useAction } from '../../../infrastructure/useAction'

const router = useRouter()
const email = ref('')
const password = ref('')
const signIn = useAction(signInMutation())

function submit() {
  signIn.run({ body: { email: email.value, password: password.value } }, () => void router.push('/profile'))
}
</script>

<template>
  <section aria-labelledby="signin-title">
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
  </section>
</template>
