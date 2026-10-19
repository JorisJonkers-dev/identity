<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { askPasswordResetMutation } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { useAction } from '../../../infrastructure/useAction'
import AuthShell from '../../../shared/AuthShell.vue'
import Icon from '../../../shared/Icon'

/** Asking for a link to choose a new password. It answers the same whether or not the email has an account. */
const email = ref('')
const ask = useAction(askPasswordResetMutation())
const emailField = ref<HTMLInputElement | null>(null)
onMounted(() => emailField.value?.focus())
</script>

<template>
  <AuthShell>
    <div v-if="ask.isDone.value" class="auth-box">
      <span class="big-ico"><Icon name="mail" /></span>
      <h1>Check your email</h1>
      <p class="lead">If {{ email }} has an account, a link to reset its password is on its way. It works for 30 minutes.</p>
      <RouterLink class="btn lg wide" to="/signin">Back to sign in</RouterLink>
    </div>
    <form v-else class="auth-box" novalidate @submit.prevent="ask.run({ body: { email } })">
      <h1>Reset your password</h1>
      <div class="field"><label class="label" for="fg-email">Email</label><input id="fg-email" ref="emailField" v-model="email" class="input" type="email" autocomplete="username" required /></div>
      <p v-if="ask.error.value" role="alert" class="alert"><Icon name="alert" />{{ ask.error.value }}</p>
      <button class="btn primary lg wide" type="submit" :disabled="ask.isPending.value">Send link</button>
      <p class="auth-foot"><RouterLink class="link" to="/signin">Back to sign in</RouterLink></p>
    </form>
  </AuthShell>
</template>
