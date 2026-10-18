<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink } from 'vue-router'
import { signUpMutation } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { useAction } from '../../../infrastructure/useAction'
import AuthLayout from '../../../shared/AuthLayout.vue'

const email = ref('')
const name = ref('')
const password = ref('')
const signUp = useAction(signUpMutation())

function submit() {
  signUp.run({ body: { email: email.value, name: name.value, password: password.value } })
}
</script>

<template>
  <AuthLayout>
    <section aria-labelledby="signup-title" class="grid gap-3.5">
      <h1 id="signup-title" class="m-0 font-display text-[26px]/[1.1] font-bold tracking-[-0.02em]">Sign up</h1>
      <p v-if="signUp.isDone.value" role="status" class="m-0 rounded-[9px] bg-ok-soft px-3 py-2 text-[13px] text-ok">Check your email for a link to finish signing up.</p>
      <form v-else class="grid gap-3.5" @submit.prevent="submit">
        <div class="grid gap-1.5">
          <label for="signup-email" class="field-label">Email</label>
          <input id="signup-email" v-model="email" class="input" type="email" autocomplete="email" required />
        </div>
        <div class="grid gap-1.5">
          <label for="signup-name" class="field-label">Name</label>
          <input id="signup-name" v-model="name" class="input" autocomplete="name" required />
        </div>
        <div class="grid gap-1.5">
          <label for="signup-password" class="field-label">Password</label>
          <input id="signup-password" v-model="password" class="input" type="password" autocomplete="new-password" minlength="12" required />
        </div>
        <p v-if="signUp.error.value" role="alert" class="alert">{{ signUp.error.value }}</p>
        <button type="submit" class="btn-primary w-full py-[11px]" :disabled="signUp.isPending.value">Sign up</button>
      </form>
      <p class="fine">Already have an account? <RouterLink to="/signin">Sign in</RouterLink></p>
    </section>
  </AuthLayout>
</template>
