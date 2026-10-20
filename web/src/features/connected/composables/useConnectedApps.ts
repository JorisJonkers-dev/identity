import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { computed } from 'vue'
import {
  listMyConnectedAppsOptions,
  listMyConnectedAppsQueryKey,
  revokeMyConnectedAppMutation,
} from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { ConnectedApp } from '../../../infrastructure/api/types.gen'
import { describeError } from '../../../infrastructure/problem'
import { useAction } from '../../../infrastructure/useAction'

/** The tools acting for the signed-in User, and disconnecting them. */
export function useConnectedApps() {
  const queryClient = useQueryClient()
  const query = useQuery({ ...listMyConnectedAppsOptions(), retry: false })
  const revoke = useAction(revokeMyConnectedAppMutation())
  return {
    // Undefined until they load.
    apps: computed<ConnectedApp[] | undefined>(() => query.data.value?.apps),
    loadError: computed(() => (query.error.value ? describeError(query.error.value) : null)),
    actionError: revoke.error,
    disconnect: (connectedAppId: string) => {
      revoke.run({ path: { connectedAppId } }, () => void queryClient.invalidateQueries({ queryKey: listMyConnectedAppsQueryKey() }))
    },
  }
}
