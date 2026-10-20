import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { computed, type Ref } from 'vue'
import { getStepUpOptions, getStepUpQueryKey, setStepUpMutation } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { StepUp } from '../../../infrastructure/api/types.gen'
import { useAction } from '../../../infrastructure/useAction'

/** How fresh an app wants two-factor (step-up), and setting it, for an Admin. */
export function useStepUp(clientId: Ref<string>) {
  const queryClient = useQueryClient()
  const options = computed(() => getStepUpOptions({ path: { clientId: clientId.value } }))
  const query = useQuery(computed(() => ({ ...options.value, retry: false })))
  const set = useAction(setStepUpMutation())
  return {
    minutes: computed(() => query.data.value?.minutes),
    // The live tokens turning step-up on would revoke.
    tokens: computed(() => query.data.value?.tokens),
    // The connected apps turning step-up on would disconnect.
    connectedApps: computed(() => query.data.value?.connectedApps),
    error: set.error,
    set: (minutes: StepUp['minutes']) => {
      set.run({ path: { clientId: clientId.value }, body: { minutes } }, () => void queryClient.invalidateQueries({ queryKey: getStepUpQueryKey({ path: { clientId: clientId.value } }) }))
    },
  }
}
