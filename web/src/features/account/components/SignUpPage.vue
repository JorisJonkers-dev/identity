<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { signUpMutation } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { useAction } from '../../../infrastructure/useAction'
import AuthShell from '../../../shared/AuthShell.vue'
import Icon from '../../../shared/Icon'

const email = ref('')
const name = ref('')
const password = ref('')
const shown = ref(false)
const short = ref(false)
const signUp = useAction(signUpMutation())
const nameField = ref<HTMLInputElement | null>(null)
onMounted(() => nameField.value?.focus())

function submit() {
  short.value = password.value.length < 12
  if (!short.value) signUp.run({ body: { email: email.value, name: name.value, password: password.value } })
}
</script>

<template>
  <AuthShell>
    <div v-if="signUp.isDone.value" class="auth-box">
      <span class="big-ico"><Icon name="mail" /></span>
      <h1>Check your email</h1>
      <p class="lead">Link sent to <b>{{ email }}</b>.</p>
      <RouterLink class="btn lg wide" to="/signin">Back to sign in</RouterLink>
    </div>
    <form v-else class="auth-box" novalidate @submit.prevent="submit">
      <h1>Create an account</h1>
      <div class="field"><label class="label" for="su-name">Name</label><input id="su-name" ref="nameField" v-model="name" class="input" autocomplete="name" required /></div>
      <div class="field"><label class="label" for="su-email">Email</label><input id="su-email" v-model="email" class="input" type="email" autocomplete="email" required /></div>
      <div class="field">
        <div class="label"><label for="su-pw">Password</label><span :class="['hint', { bad: short }]">12+ characters</span></div>
        <div class="pw">
          <input id="su-pw" v-model="password" class="input" :type="shown ? 'text' : 'password'" autocomplete="new-password" required :aria-invalid="short || undefined" aria-describedby="su-short" />
          <button type="button" :aria-label="shown ? 'Hide password' : 'Show password'" @click="shown = !shown"><Icon name="eye" /></button>
        </div>
        <p v-if="short" id="su-short" class="err"><Icon name="alert" />Use at least 12 characters.</p>
      </div>
      <p v-if="signUp.error.value" role="alert" class="alert"><Icon name="alert" />{{ signUp.error.value }}</p>
      <button class="btn primary lg wide" type="submit" :disabled="signUp.isPending.value">Create account</button>
      <p class="auth-foot">Have an account? <RouterLink class="link" to="/signin">Sign in</RouterLink></p>
    </form>
  </AuthShell>
</template>
