import { useQuery } from '@tanstack/vue-query'
import { computed } from 'vue'
import { getVersionOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { zProblem } from '../../../infrastructure/api/zod.gen'

/** What to tell the user about a failed request: the problem's detail when the server sent one. */
export function describeError(error: unknown): string {
  const problem = zProblem.safeParse(error)
  if (!problem.success) return 'Something went wrong. Try again.'
  return problem.data.detail ?? problem.data.title
}

/** The running build's version. Components read it through this and never call the API. */
export function useVersion() {
  const query = useQuery(getVersionOptions())
  return {
    version: computed(() => query.data.value?.version ?? null),
    loadError: computed(() => (query.error.value ? describeError(query.error.value) : null)),
  }
}
