<script setup lang="ts">
import { RouterLink, useRoute } from 'vue-router'
import { confirmEmailChangeMutation } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { useAction } from '../../../infrastructure/useAction'
import AuthShell from '../../../shared/AuthShell.vue'
import Icon from '../../../shared/Icon'

/** The link mailed to a new address: one click makes it the sign-in email. */
const route = useRoute()
const token = typeof route.query.token === 'string' ? route.query.token : ''
const confirm = useAction(confirmEmailChangeMutation())
</script>

<template>
  <AuthShell>
    <div v-if="!token" class="auth-box">
      <span class="big-ico warn"><Icon name="link" /></span>
      <h1>This link is incomplete</h1>
      <p class="lead">Open the full link from the email, or ask for a new one on the Account page.</p>
      <RouterLink class="btn primary lg wide" to="/account">Go to your account</RouterLink>
    </div>
    <div v-else-if="confirm.isDone.value" class="auth-box">
      <span class="big-ico ok"><Icon name="check" /></span>
      <h1>Email changed</h1>
      <p class="lead">Sign in with this address from now on.</p>
      <RouterLink class="btn primary lg wide" to="/account">Go to your account</RouterLink>
    </div>
    <div v-else class="auth-box">
      <span class="big-ico"><Icon name="mail" /></span>
      <h1>Use this email to sign in?</h1>
      <p v-if="confirm.error.value" role="alert" class="alert"><Icon name="alert" />{{ confirm.error.value }}</p>
      <button class="btn primary lg wide" type="button" :disabled="confirm.isPending.value" @click="confirm.run({ body: { token } })">Change my sign-in email</button>
    </div>
  </AuthShell>
</template>
