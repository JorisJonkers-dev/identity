import { useQuery } from '@tanstack/vue-query'
import { computed } from 'vue'
import { getLauncherOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { zProblem } from '../../../infrastructure/api/zod.gen'
import { describeError } from '../../../infrastructure/problem'

/** The apps the signed-in User can open, or that nobody is signed in. */
export function useLauncher() {
  const query = useQuery({ ...getLauncherOptions(), retry: false })
  const signedOut = computed(() => zProblem.safeParse(query.error.value).data?.status === 401)
  return {
    // Set once the apps are read: the Launcher shows only for a signed-in User.
    apps: computed(() => query.data.value?.apps),
    // Apps a Two-factor Group holds back until the User adds two-factor.
    heldBack: computed(() => query.data.value?.heldBack),
    isLoading: query.isPending,
    signedOut,
    loadError: computed(() => (query.error.value && !signedOut.value ? describeError(query.error.value) : null)),
  }
}
