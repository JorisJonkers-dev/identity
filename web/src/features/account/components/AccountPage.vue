<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink } from 'vue-router'
import AppShell from '../../../shared/AppShell.vue'
import PersonAvatar from '../../../shared/PersonAvatar.vue'
import Icon from '../../../shared/Icon'
import { useLauncher } from '../../home/composables/useLauncher'
import { useSecurity } from '../../security/composables/useSecurity'
import { useProfile } from '../composables/useProfile'

const { profile, loadError, rename } = useProfile()
const { factors } = useSecurity()
const { apps } = useLauncher()
const editing = ref(false)
const name = ref('')
const since = (at: string) => new Date(at).toLocaleDateString('en-GB', { month: 'short', year: 'numeric' })
function edit(current: string) {
  name.value = current
  editing.value = true
}
function save() {
  rename.run(name.value)
  editing.value = false
}
</script>

<template>
  <AppShell narrow>
    <p v-if="loadError" role="alert" class="alert"><Icon name="alert" />{{ loadError }}</p>
    <template v-if="profile">
      <section class="glass hero">
        <div class="hero-av"><PersonAvatar :name="profile.name" size="xxl" /></div>
        <div class="hero-t">
          <h1>{{ profile.name }}</h1>
          <p>{{ profile.email }}</p>
          <div class="pills">
            <span v-if="profile.admin" class="pill accent"><Icon name="shield" />Admin</span>
            <span v-if="apps" class="pill"><Icon name="apps" />{{ apps.length }} apps</span>
            <span class="pill">Since {{ since(profile.createdAt) }}</span>
          </div>
        </div>
      </section>
      <div class="acc-grid">
        <section class="glass card">
          <h2 class="card-t">Profile</h2>
          <form v-if="editing" class="set-row editing" @submit.prevent="save">
            <span class="set-ic"><Icon name="user" /></span>
            <div class="set-v"><label class="label" for="ac-name">Name</label><input id="ac-name" v-model="name" class="input md" autocomplete="name" required /></div>
            <div class="btn-row"><button class="btn sm ghost" type="button" @click="editing = false">Cancel</button><button class="btn sm primary" type="submit">Save</button></div>
          </form>
          <div v-else class="set-row">
            <span class="set-ic"><Icon name="user" /></span>
            <span class="set-v"><small>Name</small><b>{{ profile.name }}</b></span>
            <span v-if="rename.isDone.value" class="inline-ok"><Icon name="check" />Saved</span>
            <button class="btn sm" type="button" @click="edit(profile.name)"><Icon name="pencil" />Edit</button>
          </div>
          <p v-if="rename.error.value" role="alert" class="alert"><Icon name="alert" />{{ rename.error.value }}</p>
          <div class="set-row">
            <span class="set-ic"><Icon name="mail" /></span>
            <span class="set-v"><small>Email</small><b>{{ profile.email }}</b></span>
          </div>
        </section>
        <section class="glass card">
          <h2 class="card-t">Signing in<RouterLink class="card-link" to="/security">Security<Icon name="chevr" /></RouterLink></h2>
          <RouterLink class="set-row link" to="/security/two-factor">
            <span class="set-ic"><Icon name="shield" /></span><span class="set-v"><b>Two-factor</b></span>
            <span :class="['state', factors.length ? 'ok' : 'off']">{{ factors.length ? 'On' : 'Off' }}</span><Icon name="chevr" class="chevr" />
          </RouterLink>
          <RouterLink class="set-row link" to="/security/password">
            <span class="set-ic"><Icon name="key" /></span><span class="set-v"><b>Password</b></span><Icon name="chevr" class="chevr" />
          </RouterLink>
        </section>
      </div>
    </template>
  </AppShell>
</template>
