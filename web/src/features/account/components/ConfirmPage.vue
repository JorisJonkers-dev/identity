<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { confirmEmailMutation } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { useAction } from '../../../infrastructure/useAction'
import AuthShell from '../../../shared/AuthShell.vue'
import Icon from '../../../shared/Icon'

const route = useRoute()
const token = typeof route.query.token === 'string' ? route.query.token : ''
const password = ref('')
const shown = ref(false)
const confirm = useAction(confirmEmailMutation())
const passwordField = ref<HTMLInputElement | null>(null)
onMounted(() => passwordField.value?.focus())

// The sign-up password as well as the link: a link sent to someone else's address confirms
// nothing for whoever signed up with it.
function submit() {
  confirm.run({ body: { token, password: password.value } })
}
</script>

<template>
  <AuthShell>
    <div v-if="!token" class="auth-box">
      <span class="big-ico warn"><Icon name="link" /></span>
      <h1>This link is incomplete</h1>
      <RouterLink class="btn primary lg wide" to="/signup">Sign up again</RouterLink>
    </div>
    <div v-else-if="confirm.isDone.value" class="auth-box">
      <span class="big-ico ok"><Icon name="check" /></span>
      <h1>Email confirmed</h1>
      <RouterLink class="btn primary lg wide" to="/signin">Sign in</RouterLink>
    </div>
    <form v-else class="auth-box" novalidate @submit.prevent="submit">
      <span class="big-ico"><Icon name="mail" /></span>
      <h1>Confirm your email</h1>
      <div class="field">
        <div class="label"><label for="cf-pw">Password</label></div>
        <div class="pw">
          <input id="cf-pw" ref="passwordField" v-model="password" class="input" :type="shown ? 'text' : 'password'" autocomplete="current-password" required />
          <button type="button" :aria-label="shown ? 'Hide password' : 'Show password'" @click="shown = !shown"><Icon name="eye" /></button>
        </div>
      </div>
      <p v-if="confirm.error.value" role="alert" class="alert"><Icon name="alert" />{{ confirm.error.value }}</p>
      <button class="btn primary lg wide" type="submit" :disabled="confirm.isPending.value">Confirm email</button>
    </form>
  </AuthShell>
</template>
