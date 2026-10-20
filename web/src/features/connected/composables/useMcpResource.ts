import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { computed, type Ref } from 'vue'
import { getMcpResourceOptions, getMcpResourceQueryKey, setMcpResourceMutation } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { useAction } from '../../../infrastructure/useAction'

/** Whether an app takes connected apps, and changing it, for an Admin. */
export function useMcpResource(clientId: Ref<string>) {
  const queryClient = useQueryClient()
  const options = computed(() => getMcpResourceOptions({ path: { clientId: clientId.value } }))
  const query = useQuery(computed(() => ({ ...options.value, retry: false })))
  const set = useAction(setMcpResourceMutation())
  return {
    on: computed(() => query.data.value?.on ?? false),
    connectedApps: computed(() => query.data.value?.connectedApps ?? 0),
    tokenCheck: computed(() => query.data.value?.tokenCheck ?? 'gate'),
    error: set.error,
    set: (on: boolean) => {
      set.run({ path: { clientId: clientId.value }, body: { on } }, () => void queryClient.invalidateQueries({ queryKey: getMcpResourceQueryKey({ path: { clientId: clientId.value } }) }))
    },
  }
}
