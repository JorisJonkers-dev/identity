import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { computed, ref } from 'vue'
import {
  listMyTokensOptions,
  listMyTokensQueryKey,
  mintTokenMutation,
  revokeMyTokenMutation,
} from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { TokenDraft } from '../../../infrastructure/api/types.gen'
import { describeError } from '../../../infrastructure/problem'
import { useAction } from '../../../infrastructure/useAction'

/** The signed-in User's Personal Tokens, and minting and revoking them. */
export function useTokens() {
  const queryClient = useQueryClient()
  const query = useQuery({ ...listMyTokensOptions(), retry: false })
  const refresh = () => void queryClient.invalidateQueries({ queryKey: listMyTokensQueryKey() })
  const mint = useAction(mintTokenMutation())
  const revoke = useAction(revokeMyTokenMutation())
  // The secret of the token just made, shown once and kept nowhere else.
  const secret = ref<string | null>(null)
  return {
    tokens: computed(() => query.data.value?.tokens),
    clients: computed(() => query.data.value?.clients ?? []),
    isLoading: query.isPending,
    loadError: computed(() => (query.error.value ? describeError(query.error.value) : null)),
    actionError: computed(() => mint.error.value ?? revoke.error.value),
    secret,
    mint: (draft: TokenDraft) => {
      mint.run({ body: draft }, (made) => {
        secret.value = made.secret
        refresh()
      })
    },
    forgetSecret: () => (secret.value = null),
    revoke: (patId: string) => {
      revoke.run({ path: { patId } }, refresh)
    },
  }
}
