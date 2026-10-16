<script setup lang="ts">
import { reactive } from 'vue'
import { RouterLink } from 'vue-router'
import { useLauncher } from '../composables/useLauncher'
import { useVersion } from '../composables/useVersion'

const { version, loadError: versionError } = useVersion()
const { apps, isLoading, loadError, admin } = useLauncher()
// Apps whose icon failed to load show their label's first letter instead.
const broken = reactive(new Set<string>())
</script>

<template>
  <section v-if="apps" aria-labelledby="home-title">
    <h1 id="home-title">Your apps</h1>
    <nav aria-label="Your account" class="links">
      <RouterLink to="/profile">Your profile</RouterLink>
      <RouterLink v-if="admin" to="/admin">Dashboard</RouterLink>
      <RouterLink v-if="admin" to="/access">Manage access</RouterLink>
    </nav>
    <p v-if="apps.length === 0">You don't have access to any apps yet. Ask an Admin to add you to a Group.</p>
    <ul v-else class="apps" aria-labelledby="home-title">
      <li v-for="app in apps" :key="app.id" class="app">
        <a :href="app.launchUrl">
          <img v-if="app.iconUrl && !broken.has(app.id)" :src="app.iconUrl" alt="" width="40" height="40" @error="broken.add(app.id)" />
          <span v-else class="letter" aria-hidden="true">{{ app.label.charAt(0) }}</span>
          <span class="text">
            <span class="label">{{ app.label }}</span>
            <span v-if="app.description" class="description">{{ app.description }}</span>
            <span class="roles">Your access: {{ app.roles.join(', ') }}</span>
          </span>
        </a>
      </li>
    </ul>
  </section>
  <section v-else aria-labelledby="home-title">
    <h1 id="home-title">jorisjonkers.dev</h1>
    <p>Sign in once, and every app here knows who you are.</p>
    <p v-if="isLoading">Loading…</p>
    <p v-else-if="loadError" role="alert" class="error">{{ loadError }}</p>
    <p v-else><RouterLink to="/signin">Sign in</RouterLink> or <RouterLink to="/signup">sign up</RouterLink>.</p>
  </section>
  <p v-if="versionError" role="alert" class="error">{{ versionError }}</p>
  <footer v-else-if="version">Version {{ version }}</footer>
</template>

<style scoped>
.error {
  color: #a4161a;
}
.links {
  display: flex;
  flex-wrap: wrap;
  gap: 1rem;
  margin-bottom: 1rem;
}
.apps {
  list-style: none;
  padding: 0;
  display: grid;
  gap: 0.75rem;
}
.app a {
  display: flex;
  gap: 0.75rem;
  align-items: center;
  padding: 0.75rem;
  border: 1px solid #c8c8c8;
  border-radius: 0.75rem;
  color: inherit;
  text-decoration: none;
}
.app a:hover,
.app a:focus-visible {
  border-color: #1a1a1a;
}
.app img,
.letter {
  width: 40px;
  height: 40px;
  flex: none;
  border-radius: 0.5rem;
}
.letter {
  display: grid;
  place-items: center;
  background: #e5e9ff;
  color: #2545f0;
  font-weight: 700;
}
.text {
  display: grid;
  min-width: 0;
}
.label {
  font-weight: 600;
}
.description,
.roles {
  color: #4a4a4a;
  font-size: 0.875rem;
  overflow-wrap: anywhere;
}
footer {
  margin-top: 2rem;
  color: #4a4a4a;
  font-size: 0.875rem;
}
</style>
