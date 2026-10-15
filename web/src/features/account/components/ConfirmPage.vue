<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { confirmEmailMutation } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { useAction } from '../../../infrastructure/useAction'

const route = useRoute()
const token = typeof route.query.token === 'string' ? route.query.token : ''
const password = ref('')
const confirm = useAction(confirmEmailMutation())

// The sign-up password as well as the link: a link sent to someone else's address confirms
// nothing for whoever signed up with it.
function submit() {
  confirm.run({ body: { token, password: password.value } })
}
</script>

<template>
  <section aria-labelledby="confirm-title">
    <h1 id="confirm-title">Confirm your email</h1>
    <p v-if="!token" role="alert" class="error">This link is missing its token. Open the link from the email again.</p>
    <p v-else-if="confirm.isDone.value" role="status">
      Your email is confirmed. <RouterLink to="/signin">Sign in</RouterLink>
    </p>
    <form v-else @submit.prevent="submit">
      <p>Enter the password you chose when you signed up.</p>
      <label for="confirm-password">Password</label>
      <input id="confirm-password" v-model="password" type="password" autocomplete="current-password" required />
      <p v-if="confirm.error.value" role="alert" class="error">{{ confirm.error.value }}</p>
      <button type="submit" :disabled="confirm.isPending.value">Confirm</button>
    </form>
  </section>
</template>
