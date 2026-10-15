<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink } from 'vue-router'
import { signUpMutation } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { useAction } from '../../../infrastructure/useAction'

const email = ref('')
const name = ref('')
const password = ref('')
const signUp = useAction(signUpMutation())

function submit() {
  signUp.run({ body: { email: email.value, name: name.value, password: password.value } })
}
</script>

<template>
  <section aria-labelledby="signup-title">
    <h1 id="signup-title">Sign up</h1>
    <p v-if="signUp.isDone.value" role="status">Check your email for a link to finish signing up.</p>
    <form v-else @submit.prevent="submit">
      <label for="signup-email">Email</label>
      <input id="signup-email" v-model="email" type="email" autocomplete="email" required />
      <label for="signup-name">Name</label>
      <input id="signup-name" v-model="name" autocomplete="name" required />
      <label for="signup-password">Password</label>
      <input id="signup-password" v-model="password" type="password" autocomplete="new-password" minlength="12" required />
      <p v-if="signUp.error.value" role="alert" class="error">{{ signUp.error.value }}</p>
      <button type="submit" :disabled="signUp.isPending.value">Sign up</button>
    </form>
    <p>Already have an account? <RouterLink to="/signin">Sign in</RouterLink></p>
  </section>
</template>
