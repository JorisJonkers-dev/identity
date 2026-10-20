<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import AppShell from '../../../shared/AppShell.vue'
import Icon from '../../../shared/Icon'
import { useSecurity } from '../composables/useSecurity'

const { factors, loadError } = useSecurity()
const kinds = computed(() => {
  const k = new Set(factors.value.map((f) => f.kind))
  const said = [k.has('passkey') && 'passkey', k.has('totp') && 'authenticator app'].filter(Boolean).join(' and ')
  return said.charAt(0).toUpperCase() + said.slice(1)
})
const latest = computed(() => factors.value.at(-1))
function day(at: string): string {
  return new Date(at).toLocaleDateString('en-GB', { day: 'numeric', month: 'short' })
}
</script>

<template>
  <AppShell narrow>
    <header class="head"><h1>Security</h1></header>
    <p v-if="loadError" role="alert" class="alert"><Icon name="alert" />{{ loadError }}</p>
    <div class="glass facts3">
      <div><span>Two-factor</span><b :class="factors.length ? 'okc' : 'offc'">{{ factors.length ? 'On' : 'Off' }}</b><small>{{ factors.length }} {{ factors.length === 1 ? 'method' : 'methods' }}</small></div>
      <div><span>Last added</span><b>{{ latest?.label ?? 'Nothing yet' }}</b><small v-if="latest">{{ day(latest.createdAt) }}</small></div>
      <div><span>Password</span><b>Set</b><small>Change any time</small></div>
    </div>
    <section class="glass group">
      <RouterLink class="row link-row" to="/security/two-factor">
        <span class="row-ic"><Icon name="shield" /></span>
        <span class="grow"><b>Two-factor</b><small>{{ kinds || 'Off' }}</small></span>
        <span :class="['state', factors.length ? 'ok' : 'off']">{{ factors.length ? 'On' : 'Off' }}</span>
        <Icon name="chevr" class="chevr" />
      </RouterLink>
      <RouterLink class="row link-row" to="/security/devices">
        <span class="row-ic"><Icon name="monitor" /></span>
        <span class="grow"><b>Devices</b><small>Where you are signed in</small></span>
        <Icon name="chevr" class="chevr" />
      </RouterLink>
      <RouterLink class="row link-row" to="/security/connected-apps">
        <span class="row-ic"><Icon name="bot" /></span>
        <span class="grow"><b>Connected apps</b><small>Tools that use your apps as you</small></span>
        <Icon name="chevr" class="chevr" />
      </RouterLink>
      <RouterLink class="row link-row" to="/security/password">
        <span class="row-ic"><Icon name="key" /></span>
        <span class="grow"><b>Password</b><small>Other devices are signed out when it changes</small></span>
        <Icon name="chevr" class="chevr" />
      </RouterLink>
    </section>
    <section class="glass group">
      <RouterLink class="row link-row" to="/security/delete">
        <span class="row-ic danger-t"><Icon name="trash" /></span>
        <span class="grow"><b>Delete account</b><small>Ends your access to every app</small></span>
        <Icon name="chevr" class="chevr" />
      </RouterLink>
    </section>
  </AppShell>
</template>
