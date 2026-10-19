<script setup lang="ts">
import { adminMail } from '../../../shared/contact'
import { computed, ref } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import type { SecondFactor } from '../../../infrastructure/api/types.gen'
import AppShell from '../../../shared/AppShell.vue'
import BackLink from '../../../shared/BackLink.vue'
import Icon from '../../../shared/Icon'
import DialogScrim from '../../../shared/DialogScrim.vue'
import { useSecurity } from '../composables/useSecurity'

const route = useRoute()
const forAdmin = computed(() => route.query.for === 'admin')
const forApp = computed(() => route.query.for === 'step-up')
const { factors, loadError, passkeysSupported, addPasskey, remove } = useSecurity()
const removing = ref<SecondFactor | null>(null)
const adding = ref(false)
const password = ref('')
const label = ref('')
const hasApp = computed(() => factors.value.some((f) => f.kind === 'totp'))

function day(at: string): string {
  return new Date(at).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
}
function close() {
  removing.value = null
  adding.value = false
  password.value = ''
  label.value = ''
}
function confirmRemove(factor: SecondFactor) {
  remove.run(factor.id, password.value)
  close()
}
function confirmAdd() {
  addPasskey.run(label.value, password.value, close)
}
</script>

<template>
  <AppShell narrow>
    <BackLink to="/security" label="Security" />
    <header class="head"><h1>Two-factor</h1></header>
    <p v-if="forAdmin" class="note warn" role="status"><Icon name="shield" />Admin pages need two-factor. Add a passkey or an authenticator app, then sign in again.</p>
    <p v-if="forApp" class="note warn" role="status"><Icon name="shield" />The app you opened needs two-factor. Add a passkey or an authenticator app, then open the app again.</p>
    <p v-if="loadError" role="alert" class="alert"><Icon name="alert" />{{ loadError }}</p>
    <div class="task">
      <div class="task-main">
        <section v-if="factors.length" class="glass group">
          <div v-for="f in factors" :key="f.id" class="row">
            <span class="row-ic"><Icon :name="f.kind === 'passkey' ? 'passkey' : 'phonekey'" /></span>
            <span class="grow">
              <b>{{ f.kind === 'passkey' ? 'Passkey' : 'Authenticator app' }}</b>
              <small>{{ f.label }} · {{ f.lastUsedAt ? `used ${day(f.lastUsedAt)}` : `added ${day(f.createdAt)}` }}</small>
            </span>
            <RouterLink v-if="f.kind === 'totp'" class="btn ghost sm" to="/security/two-factor/setup">Replace</RouterLink>
            <button class="btn ghost sm" type="button" @click="removing = f">Remove</button>
          </div>
        </section>
        <section v-else class="glass off-box">
          <span class="big-ico warn"><Icon name="shield" /></span>
          <div><b>Two-factor is off</b><p>A password alone gets into your account.</p></div>
        </section>
        <p v-if="remove.error.value || addPasskey.error.value" role="alert" class="alert"><Icon name="alert" />{{ remove.error.value ?? addPasskey.error.value }}</p>
        <div class="btn-row">
          <RouterLink v-if="!hasApp" class="btn primary" to="/security/two-factor/setup"><Icon name="phonekey" />Set up an authenticator app</RouterLink>
          <button v-if="passkeysSupported" class="btn" type="button" @click="adding = true"><Icon name="plus" />Add a passkey</button>
        </div>
      </div>
      <aside class="task-aside glass"><b>Lost your phone?</b><a class="btn sm" :href="adminMail"><Icon name="mail" />Contact an Admin</a></aside>
    </div>

    <DialogScrim v-if="removing" @close="close">
      <form class="dialog glass strong" role="dialog" aria-modal="true" aria-labelledby="rm-h" @submit.prevent="confirmRemove(removing)">
        <h2 id="rm-h">Remove {{ removing.kind === 'passkey' ? 'this passkey' : 'the authenticator app' }}?</h2>
        <div class="field"><label class="label" for="rm-pw">Password</label><input id="rm-pw" v-model="password" class="input md" type="password" autocomplete="current-password" required /></div>
        <div class="btn-row end"><button class="btn" type="button" @click="close">Cancel</button><button class="btn danger-solid" type="submit" :disabled="remove.isPending.value">Remove</button></div>
      </form>
    </DialogScrim>
    <DialogScrim v-if="adding" @close="close">
      <form class="dialog glass strong" role="dialog" aria-modal="true" aria-labelledby="pk-h" @submit.prevent="confirmAdd">
        <h2 id="pk-h">Add a passkey</h2>
        <div class="field"><label class="label" for="pk-l">Name</label><input id="pk-l" v-model="label" class="input md" placeholder="MacBook Pro" required /></div>
        <div class="field"><label class="label" for="pk-pw">Password</label><input id="pk-pw" v-model="password" class="input md" type="password" autocomplete="current-password" required /></div>
        <div class="btn-row end"><button class="btn" type="button" @click="close">Cancel</button><button class="btn primary" type="submit" :disabled="addPasskey.isPending.value">Add passkey</button></div>
      </form>
    </DialogScrim>
  </AppShell>
</template>
