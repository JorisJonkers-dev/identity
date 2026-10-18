<script setup lang="ts">
import { RouterLink } from 'vue-router'
import CardLayout from '../../../shared/CardLayout.vue'
</script>

<template>
  <CardLayout glyph="stop">
    <section aria-labelledby="no-access-title" class="grid gap-3.5">
      <h1 id="no-access-title" class="m-0 font-display text-[22px]/[1.15] font-bold tracking-[-0.02em]">You don't have access to this app</h1>
      <p class="fine">You are signed in, but none of your Groups has a role in the app you came from. Ask an Admin to add you to a Group that does.</p>
      <div class="flex flex-wrap gap-2">
        <RouterLink to="/" class="btn-primary flex-1 text-center">Open your apps</RouterLink>
        <RouterLink to="/profile" class="btn flex-1 text-center">Go to your profile</RouterLink>
      </div>
    </section>
  </CardLayout>
</template>
