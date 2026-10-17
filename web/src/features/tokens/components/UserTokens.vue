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
  <div class="mt-2 grid gap-1.5" :aria-label="`${name}'s tokens`" role="group">
    <p v-if="error" role="alert" class="m-0 text-bad">{{ error }}</p>
    <p v-else-if="query.data.value?.tokens.length === 0" class="m-0 text-ink-3">{{ name }} has no tokens.</p>
    <ul v-else-if="query.data.value" class="m-0 grid list-none gap-1.5 p-0">
      <li v-for="t in query.data.value.tokens" :key="t.id" class="flex flex-wrap items-center gap-2" :aria-label="t.name">
        <span><b>{{ t.name }}</b> · {{ t.client.label }} · {{ expiry(t, now) }}</span>
        <template v-if="t.state === 'active' || t.state === 'expiring'">
          <template v-if="asking === t.id">
            <span>Revoke {{ t.name }}?</span>
            <button type="button" @click="confirm(t.id)">Yes, revoke</button>
            <button type="button" @click="asking = null">Cancel</button>
          </template>
          <button v-else type="button" :aria-label="`Revoke ${t.name}`" @click="asking = t.id">Revoke</button>
        </template>
      </li>
    </ul>
  </div>
</template>
