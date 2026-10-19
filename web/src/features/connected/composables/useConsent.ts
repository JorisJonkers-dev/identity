import { useMutation, useQuery } from '@tanstack/vue-query'
import { computed } from 'vue'
import { client } from '../../../infrastructure/api/client.gen'
import { zProblem } from '../../../infrastructure/api/zod.gen'
import { describeError } from '../../../infrastructure/problem'

/** What a tool asks to do for the User, as the Identity Provider's consent endpoint answers it. */
export interface Prompt {
  app: string
  client: string
  roles: string[]
  redirectHost: string
}

// The consent endpoint is the Identity Provider's, beside /oidc/login, not part of the API.
const url = '/oidc/consent'

/** A tool's request waiting for the User's consent, and answering it. */
export function useConsent(authRequestID: string) {
  const query = useQuery({
    queryKey: ['consent', authRequestID],
    queryFn: async () => (await client.get({ url, query: { authRequestID }, throwOnError: true })).data as Prompt,
    retry: false,
  })
  const answer = useMutation({
    mutationFn: async (allow: boolean) => (await client.post({ url, body: { authRequestID, allow }, throwOnError: true })).data as { redirect: string },
  })
  return {
    prompt: computed(() => query.data.value),
    loadError: computed(() => (query.error.value ? describeError(query.error.value) : null)),
    // 401 when the User is not signed in yet.
    loadStatus: computed(() => zProblem.safeParse(query.error.value).data?.status ?? null),
    answerError: computed(() => (answer.error.value ? describeError(answer.error.value) : null)),
    answering: answer.isPending,
    answer: (allow: boolean, then: (redirect: string) => void) => {
      answer.mutate(allow, { onSuccess: (next) => { then(next.redirect) } })
    },
  }
}
