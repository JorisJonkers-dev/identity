<script setup lang="ts">
import { adminMail } from '../../../shared/contact'
import { RouterLink } from 'vue-router'
import AuthShell from '../../../shared/AuthShell.vue'
import Icon from '../../../shared/Icon'
</script>

<template>
  <AuthShell>
    <div class="auth-box">
      <span class="big-ico warn"><Icon name="lock" /></span>
      <h1>No access to this app</h1>
      <RouterLink class="btn primary lg wide" to="/">Your apps</RouterLink>
      <p class="auth-foot"><a class="link" :href="adminMail">Contact an Admin</a></p>
    </div>
  </AuthShell>
</template>
