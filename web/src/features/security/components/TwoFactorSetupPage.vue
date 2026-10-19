<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import AppShell from '../../../shared/AppShell.vue'
import CodeBoxes from '../../../shared/CodeBoxes.vue'
import BackLink from '../../../shared/BackLink.vue'
import Icon from '../../../shared/Icon'
import { useCopy } from '../../../shared/useCopy'
import { useSecurity } from '../composables/useSecurity'
import QrCode from './QrCode.vue'

const { setup, start, confirm } = useSecurity()
const steps = [
  { key: 'scan', label: 'Scan' },
  { key: 'code', label: 'Code' },
  { key: 'password', label: 'Password' },
] as const
type Step = (typeof steps)[number]['key'] | 'done'
const step = ref<Step>('scan')
const code = ref('')
const label = ref('Phone')
const password = ref('')
const { copied, copy } = useCopy()
onMounted(() => { start.run(); })
const codeBoxes = ref<{ focus: () => void } | null>(null)
watch(step, (s) => {
  if (s === 'code') void nextTick(() => codeBoxes.value?.focus())
})

// The steps show only until the last is done.
const index = (s: Step) => steps.findIndex((x) => x.key === s)
function finish() {
  confirm.run(label.value, code.value, password.value, () => (step.value = 'done'))
}
</script>

<template>
  <AppShell narrow>
    <BackLink to="/security/two-factor" label="Two-factor" />
    <header class="head"><h1>Set up two-factor</h1></header>
    <ol v-if="step !== 'done'" class="steps">
      <li v-for="(s, i) in steps" :key="s.key" :class="{ on: s.key === step, done: i < index(step) }" :aria-current="s.key === step ? 'step' : undefined">
        <span class="n"><Icon v-if="i < index(step)" name="check" /><template v-else>{{ i + 1 }}</template></span>{{ s.label }}
      </li>
    </ol>
    <p v-if="start.error.value" role="alert" class="alert"><Icon name="alert" />{{ start.error.value }}</p>
    <div v-if="step === 'scan'" class="glass task-form">
      <h2>Scan this with your authenticator app</h2>
      <div v-if="setup" class="qr-row">
        <div class="qr"><QrCode :value="setup.uri" label="QR code for your authenticator app" /></div>
        <div class="qr-side">
          <span class="label">Can't scan? Type this key</span>
          <div class="secret"><code>{{ setup.secret }}</code><button class="btn sm" type="button" @click="copy(setup.secret)"><Icon name="copy" />{{ copied ? 'Copied' : 'Copy' }}</button></div>
        </div>
      </div>
      <div class="form-save"><button class="btn primary" type="button" :disabled="!setup" @click="step = 'code'">I have scanned it</button></div>
    </div>
    <form v-else-if="step === 'code'" class="glass task-form" novalidate @submit.prevent="step = 'password'">
      <h2>Enter the code your app shows</h2>
      <CodeBoxes ref="codeBoxes" v-model="code" />
      <div class="field"><label class="label" for="tf-l">Name</label><input id="tf-l" v-model="label" class="input md" required /></div>
      <div class="form-save"><button class="btn ghost" type="button" @click="step = 'scan'">Back</button><button class="btn primary" type="submit" :disabled="code.length < 6">Continue</button></div>
    </form>
    <form v-else-if="step === 'password'" class="glass task-form" novalidate @submit.prevent="finish">
      <h2>Re-enter your password</h2>
      <div class="field"><label class="label" for="tf-pw">Password</label><input id="tf-pw" v-model="password" class="input md" type="password" autocomplete="current-password" required /></div>
      <p v-if="confirm.error.value" role="alert" class="alert"><Icon name="alert" />{{ confirm.error.value }}</p>
      <div class="form-save"><button class="btn ghost" type="button" @click="step = 'code'">Back</button><button class="btn primary" type="submit" :disabled="confirm.isPending.value">Turn on two-factor</button></div>
    </form>
    <div v-else class="glass off-box">
      <span class="big-ico ok"><Icon name="check" /></span>
      <div><b>Two-factor is on</b><p>Your app asks for a code at every sign-in.</p></div>
      <div class="btn-row"><RouterLink class="btn primary" to="/security">Back to security</RouterLink></div>
    </div>
  </AppShell>
</template>
