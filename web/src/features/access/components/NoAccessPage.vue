<script setup lang="ts">
import { RouterLink } from 'vue-router'
</script>

<template>
  <section aria-labelledby="no-access-title">
    <h1 id="no-access-title">No access to this app yet</h1>
    <p>You are signed in, but none of your Groups gives you a role in the app you came from. Ask an Admin to give you access.</p>
    <p><RouterLink to="/profile">Your profile</RouterLink></p>
  </section>
</template>
