<script setup lang="ts">
import { RouterLink } from 'vue-router'
</script>

<template>
  <section aria-labelledby="no-access-title">
    <h1 id="no-access-title">You don't have access to this app</h1>
    <p>You are signed in, but none of your Groups has a role in the app you came from. Ask an Admin to add you to a Group that does.</p>
    <p><RouterLink to="/profile">Go to your profile</RouterLink></p>
  </section>
</template>
