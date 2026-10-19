<script setup lang="ts">
import { adminMail } from '../../../shared/contact'
import { ref } from 'vue'
import AppShell from '../../../shared/AppShell.vue'
import BackLink from '../../../shared/BackLink.vue'
import Icon from '../../../shared/Icon'
import { useProfile } from '../../account/composables/useProfile'

const { changePassword } = useProfile()
const current = ref('')
const next = ref('')
const short = ref(false)
function submit() {
  short.value = next.value.length < 12
  if (short.value) return
  changePassword.run({ body: { current: current.value, next: next.value } }, () => {
    current.value = ''
    next.value = ''
  })
}
</script>

<template>
  <AppShell narrow>
    <BackLink to="/security" label="Security" />
    <header class="head"><h1>Password</h1></header>
    <div class="task">
      <form class="glass task-form task-main" novalidate @submit.prevent="submit">
        <p v-if="changePassword.isDone.value" class="note" role="status"><Icon name="check" />Password changed. Other devices are signed out.</p>
        <div class="field"><label class="label" for="pw-cur">Current password</label><input id="pw-cur" v-model="current" class="input md" type="password" autocomplete="current-password" required /></div>
        <div class="field">
          <div class="label"><label for="pw-new">New password</label><span :class="['hint', { bad: short }]">12+ characters</span></div>
          <input id="pw-new" v-model="next" class="input md" type="password" autocomplete="new-password" required :aria-invalid="short || undefined" />
          <p v-if="short" class="err"><Icon name="alert" />Use at least 12 characters.</p>
        </div>
        <p v-if="changePassword.error.value" role="alert" class="alert"><Icon name="alert" />{{ changePassword.error.value }}</p>
        <div class="form-save"><span class="hint">Other devices are signed out.</span><button class="btn primary" type="submit" :disabled="changePassword.isPending.value">Change password</button></div>
      </form>
      <aside class="task-aside glass"><b>Forgot it?</b><a class="btn sm" :href="adminMail"><Icon name="mail" />Contact an Admin</a></aside>
    </div>
  </AppShell>
</template>
