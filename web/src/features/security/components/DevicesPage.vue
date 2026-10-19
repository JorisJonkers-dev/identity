<script setup lang="ts">
import AppShell from '../../../shared/AppShell.vue'
import BackLink from '../../../shared/BackLink.vue'
import Icon from '../../../shared/Icon'
import { ago } from '../../admin/format'
import { useDevices } from '../composables/useDevices'

const { devices, loadError, actionError, signOut, signOutOthers } = useDevices()
const now = new Date()
const phone = (label: string) => / on (iOS|iPadOS|Android)$/.test(label)
</script>

<template>
  <AppShell narrow>
    <BackLink to="/security" label="Security" />
    <header class="head"><h1>Devices</h1></header>
    <p v-if="loadError || actionError" role="alert" class="alert"><Icon name="alert" />{{ loadError ?? actionError }}</p>
    <template v-if="devices">
      <section class="glass group">
        <div v-for="d in devices" :key="d.id" class="row device">
          <span class="row-ic"><Icon :name="phone(d.label) ? 'phone' : 'laptop'" /></span>
          <span class="grow"><b>{{ d.label }}</b><small>{{ d.current ? 'This device' : `Last used ${ago(d.lastSeenAt, now)}` }}</small></span>
          <button v-if="!d.current" class="btn sm" type="button" @click="signOut(d.id)">Sign out</button>
        </div>
      </section>
      <div class="btn-row">
        <button v-if="devices.some((d) => !d.current)" class="btn danger-t" type="button" @click="signOutOthers"><Icon name="logout" />Sign out all other devices</button>
        <p v-else class="empty-state">You are signed in on this device only.</p>
      </div>
    </template>
  </AppShell>
</template>
