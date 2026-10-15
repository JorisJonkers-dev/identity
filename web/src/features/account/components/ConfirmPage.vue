<script setup lang="ts">
import { onMounted } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { confirmEmailMutation } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { useAction } from '../../../infrastructure/useAction'

const route = useRoute()
const confirm = useAction(confirmEmailMutation())
const token = typeof route.query.token === 'string' ? route.query.token : ''

onMounted(() => {
  if (token) confirm.run({ body: { token } })
})
</script>

<template>
  <section aria-labelledby="confirm-title">
    <h1 id="confirm-title">Confirm your email</h1>
    <p v-if="!token" role="alert" class="error">This link is missing its token. Open the link from the email again.</p>
    <p v-else-if="confirm.isDone.value" role="status">
      Your email is confirmed. <RouterLink to="/signin">Sign in</RouterLink>
    </p>
    <p v-else-if="confirm.error.value" role="alert" class="error">{{ confirm.error.value }}</p>
    <p v-else>Confirming…</p>
  </section>
</template>
