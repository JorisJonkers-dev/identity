<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { confirmEmailMutation } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { useAction } from '../../../infrastructure/useAction'
import CardLayout from '../../../shared/CardLayout.vue'

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
  <CardLayout glyph="mail">
    <section aria-labelledby="confirm-title" class="grid gap-3.5">
      <h1 id="confirm-title" class="page-title">Confirm your email</h1>
      <p v-if="!token" role="alert" class="alert">This link is incomplete. Open the link in your email again.</p>
      <p v-else-if="confirm.isDone.value" role="status" class="m-0 rounded-[9px] bg-ok-soft px-3 py-2 text-[13px] text-ok">
        Your email is confirmed. <RouterLink to="/signin" class="font-semibold text-ok">Sign in</RouterLink>
      </p>
      <form v-else class="grid gap-3.5" @submit.prevent="submit">
        <p class="fine">Enter the password you chose when you signed up.</p>
        <div class="grid gap-1.5">
          <label for="confirm-password" class="field-label">Password</label>
          <input id="confirm-password" v-model="password" class="input" type="password" autocomplete="current-password" required />
        </div>
        <p v-if="confirm.error.value" role="alert" class="alert">{{ confirm.error.value }}</p>
        <button type="submit" class="btn-primary w-full py-[11px]" :disabled="confirm.isPending.value">Confirm email</button>
      </form>
    </section>
  </CardLayout>
</template>
