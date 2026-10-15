import { useQuery } from '@tanstack/vue-query'
import { computed } from 'vue'
import { getVersionOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { describeError } from '../../../infrastructure/problem'

/** The running build's version. Components read it through this and never call the API. */
export function useVersion() {
  const query = useQuery(getVersionOptions())
  return {
    version: computed(() => query.data.value?.version ?? null),
    loadError: computed(() => (query.error.value ? describeError(query.error.value) : null)),
  }
}
