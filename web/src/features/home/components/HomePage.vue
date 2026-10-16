<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { useVersion } from '../composables/useVersion'

const { version, loadError } = useVersion()
</script>

<template>
  <section aria-labelledby="home-title">
    <h1 id="home-title">jorisjonkers.dev</h1>
    <p>Sign in once, and every app here knows who you are.</p>
    <p><RouterLink to="/signin">Sign in</RouterLink> or <RouterLink to="/signup">sign up</RouterLink>.</p>
    <p v-if="loadError" role="alert" class="error">{{ loadError }}</p>
    <footer v-else-if="version">Version {{ version }}</footer>
  </section>
</template>

<style scoped>
.error {
  color: #a4161a;
}
footer {
  margin-top: 2rem;
  color: #4a4a4a;
  font-size: 0.875rem;
}
</style>
