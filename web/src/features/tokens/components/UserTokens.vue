<script setup lang="ts">
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { computed, ref } from 'vue'
import {
  listUserTokensOptions,
  listUserTokensQueryKey,
  revokeUserTokenMutation,
} from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { describeError } from '../../../infrastructure/problem'
import { useAction } from '../../../infrastructure/useAction'
import AppIcon from '../../../shared/AppIcon.vue'
import { expiry } from '../expiry'

// A User's Personal Tokens, for an Admin, who can revoke any live one.
const props = defineProps<{ userId: string; name: string }>()
const queryClient = useQueryClient()
const options = computed(() => listUserTokensOptions({ path: { userId: props.userId } }))
const query = useQuery(computed(() => ({ ...options.value, retry: false })))
const revoke = useAction(revokeUserTokenMutation())
const asking = ref<string | null>(null)
const now = new Date()
const error = computed(() => (query.error.value ? describeError(query.error.value) : revoke.error.value))

function confirm(patId: string) {
  asking.value = null
  revoke.run({ path: { userId: props.userId, patId } }, () => {
    void queryClient.invalidateQueries({ queryKey: listUserTokensQueryKey({ path: { userId: props.userId } }) })
  })
}
</script>

<template>
  <div class="user-tokens" :aria-label="`${name}'s tokens`" role="group">
    <p v-if="error" role="alert" class="err-line">{{ error }}</p>
    <p v-else-if="query.data.value?.tokens.length === 0" class="soft">No tokens</p>
    <div v-for="t in query.data.value?.tokens ?? []" :key="t.id" class="ut-row" :aria-label="t.name">
      <AppIcon :label="t.client.label" size="sm" />
      <span class="grow"><b class="mono">{{ t.name }}</b><small>{{ t.client.label }} · {{ expiry(t, now) }}</small></span>
      <template v-if="t.state === 'active' || t.state === 'expiring'">
        <template v-if="asking === t.id">
          <button class="btn sm" type="button" @click="asking = null">Cancel</button>
          <button class="btn sm danger-solid" type="button" @click="confirm(t.id)">Revoke</button>
        </template>
        <button v-else class="btn ghost sm" type="button" :aria-label="`Revoke ${t.name}`" @click="asking = t.id">Revoke</button>
      </template>
    </div>
  </div>
</template>
