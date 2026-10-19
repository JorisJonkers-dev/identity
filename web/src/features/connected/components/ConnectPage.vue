<script setup lang="ts">
import { watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { navigation } from '../../../infrastructure/navigation'
import AuthShell from '../../../shared/AuthShell.vue'
import Icon from '../../../shared/Icon'
import { useConsent } from '../composables/useConsent'

/** A tool asks to act for the signed-in User on one app: they connect it, or cancel. */
const route = useRoute()
const router = useRouter()
const authRequestID = typeof route.query.authRequestID === 'string' ? route.query.authRequestID : ''
const { prompt, loadError, loadStatus, answerError, answering, answer } = useConsent(authRequestID)
watch(
  loadStatus,
  (status) => {
    if (status === 401) void router.replace({ path: '/signin', query: { authRequestID } })
  },
  { immediate: true },
)
// The Identity Provider sends the browser on: to the tool with its code, or with the refusal.
function reply(allow: boolean) {
  answer(allow, (redirect) => { navigation.leave(redirect) })
}
</script>

<template>
  <AuthShell>
    <div v-if="prompt" class="auth-box">
      <span class="big-ico"><Icon name="bot" /></span>
      <h1>Connect {{ prompt.app }} to {{ prompt.client }}?</h1>
      <p class="lead">{{ prompt.app }} can then use {{ prompt.client }} as you, with your roles there:</p>
      <div class="flex flex-wrap justify-center gap-1.5"><span v-for="r in prompt.roles" :key="r" class="role">{{ r }}</span></div>
      <p class="text-center text-sm text-[var(--ink-3)]">The tool chose the name {{ prompt.app }} itself. After you connect, it continues at {{ prompt.redirectHost }}. You can disconnect it any time under Security.</p>
      <p v-if="answerError" role="alert" class="alert"><Icon name="alert" />{{ answerError }}</p>
      <button class="btn primary lg wide" type="button" :disabled="answering" @click="reply(true)">Connect</button>
      <button class="btn lg wide" type="button" :disabled="answering" @click="reply(false)">Cancel</button>
    </div>
    <div v-else-if="loadError && loadStatus !== 401" class="auth-box">
      <span class="big-ico warn"><Icon name="alert" /></span>
      <h1>This tool cannot connect</h1>
      <p role="alert" class="alert"><Icon name="alert" />{{ loadError }}</p>
      <RouterLink class="btn primary lg wide" to="/">Your apps</RouterLink>
    </div>
  </AuthShell>
</template>
