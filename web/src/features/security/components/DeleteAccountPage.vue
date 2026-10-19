<script setup lang="ts">
import { ref, watch } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { deleteAccountMutation } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { useAction } from '../../../infrastructure/useAction'
import AppShell from '../../../shared/AppShell.vue'
import AuthShell from '../../../shared/AuthShell.vue'
import BackLink from '../../../shared/BackLink.vue'
import Icon from '../../../shared/Icon'

/** Deleting the signed-in User's account, with their password and, if they have it, a fresh two-factor proof. */
const router = useRouter()
const password = ref('')
const remove = useAction(deleteAccountMutation())
// The server asks for a fresh two-factor proof: the step-up page takes it, then comes back here.
watch(remove.status, (status) => {
  if (status === 403) void router.push('/signin/step-up?next=/security/delete')
})
</script>

<template>
  <AuthShell v-if="remove.isDone.value">
    <div class="auth-box">
      <span class="big-ico ok"><Icon name="check" /></span>
      <h1>Account deleted</h1>
      <RouterLink class="btn primary lg wide" to="/">Go to the start page</RouterLink>
    </div>
  </AuthShell>
  <AppShell v-else narrow>
    <BackLink to="/security" label="Security" />
    <header class="head"><h1>Delete your account</h1></header>
    <p class="note warn" role="status"><Icon name="alert" />You are signed out everywhere, your tokens stop working and your access to every app ends. This cannot be undone.</p>
    <form class="glass group task" @submit.prevent="remove.run({ body: { password } })">
      <div class="field"><label class="label" for="del-pw">Password</label><input id="del-pw" v-model="password" class="input md" type="password" autocomplete="current-password" required /></div>
      <p v-if="remove.error.value && remove.status.value !== 403" role="alert" class="alert"><Icon name="alert" />{{ remove.error.value }}</p>
      <div class="btn-row end"><RouterLink class="btn" to="/security">Cancel</RouterLink><button class="btn danger-solid" type="submit" :disabled="remove.isPending.value">Delete my account</button></div>
    </form>
  </AppShell>
</template>
