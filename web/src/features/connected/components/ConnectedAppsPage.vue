<script setup lang="ts">
import AppShell from '../../../shared/AppShell.vue'
import BackLink from '../../../shared/BackLink.vue'
import Icon from '../../../shared/Icon'
import { ago } from '../../admin/format'
import { useConnectedApps } from '../composables/useConnectedApps'

const { apps, loadError, actionError, disconnect } = useConnectedApps()
const now = new Date()
</script>

<template>
  <AppShell narrow>
    <BackLink to="/security" label="Security" />
    <header class="head"><h1>Connected apps</h1></header>
    <p v-if="loadError || actionError" role="alert" class="alert"><Icon name="alert" />{{ loadError ?? actionError }}</p>
    <template v-if="apps">
      <section v-if="apps.length" class="glass group">
        <div v-for="a in apps" :key="a.id" class="row">
          <span class="row-ic"><Icon name="bot" /></span>
          <span class="grow"><b>{{ a.name }}</b><small>Uses {{ a.client.label }} as you · {{ a.lastUsedAt ? `Last used ${ago(a.lastUsedAt, now)}` : 'Not used yet' }}</small></span>
          <button class="btn sm" type="button" @click="disconnect(a.id)">Disconnect</button>
        </div>
      </section>
      <p v-else class="empty-state">No app acts for you. An app asks before it connects.</p>
    </template>
  </AppShell>
</template>
