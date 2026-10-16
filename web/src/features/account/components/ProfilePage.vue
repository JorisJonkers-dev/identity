<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { useProfile } from '../composables/useProfile'

const router = useRouter()
const { profile, isLoading, loadError, rename, changePassword, signOut } = useProfile()
const name = ref('')
const current = ref('')
const next = ref('')

function submitName() {
  rename.run(name.value)
}

function submitPassword() {
  changePassword.run({ body: { current: current.value, next: next.value } }, () => {
    current.value = ''
    next.value = ''
  })
}
</script>

<template>
  <section aria-labelledby="profile-title">
    <h1 id="profile-title">Your profile</h1>
    <p v-if="isLoading">Loading…</p>
    <p v-else-if="loadError" role="alert" class="error">
      {{ loadError }} <RouterLink to="/signin">Sign in</RouterLink>
    </p>
    <template v-else-if="profile">
      <dl>
        <dt>Name</dt>
        <dd>{{ profile.name }}</dd>
        <dt>Email</dt>
        <dd>{{ profile.email }}</dd>
      </dl>
      <p v-if="profile.admin"><RouterLink to="/access">Manage access</RouterLink></p>

      <form aria-labelledby="rename-title" @submit.prevent="submitName">
        <h2 id="rename-title">Change your name</h2>
        <label for="profile-name">New name</label>
        <input id="profile-name" v-model="name" autocomplete="name" required />
        <p v-if="rename.error.value" role="alert" class="error">{{ rename.error.value }}</p>
        <button type="submit" :disabled="rename.isPending.value">Save name</button>
      </form>

      <form aria-labelledby="password-title" @submit.prevent="submitPassword">
        <h2 id="password-title">Change your password</h2>
        <label for="profile-current">Current password</label>
        <input id="profile-current" v-model="current" type="password" autocomplete="current-password" required />
        <label for="profile-next">New password</label>
        <input id="profile-next" v-model="next" type="password" autocomplete="new-password" minlength="12" required />
        <p v-if="changePassword.error.value" role="alert" class="error">{{ changePassword.error.value }}</p>
        <p v-else-if="changePassword.isDone.value" role="status">Your password has been changed. Your other sessions are signed out.</p>
        <button type="submit" :disabled="changePassword.isPending.value">Change password</button>
      </form>

      <button type="button" :disabled="signOut.isPending.value" @click="signOut.run({}, () => void router.push('/signin'))">Sign out</button>
    </template>
  </section>
</template>
