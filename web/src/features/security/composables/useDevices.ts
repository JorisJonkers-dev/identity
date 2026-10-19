import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { computed } from 'vue'
import {
  listDevicesOptions,
  listDevicesQueryKey,
  signOutDeviceMutation,
  signOutOtherDevicesMutation,
} from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { Device } from '../../../infrastructure/api/types.gen'
import { describeError } from '../../../infrastructure/problem'
import { useAction } from '../../../infrastructure/useAction'

/** Where the signed-in User is signed in, and signing those places out. */
export function useDevices() {
  const queryClient = useQueryClient()
  const query = useQuery({ ...listDevicesOptions(), retry: false })
  const refresh = () => void queryClient.invalidateQueries({ queryKey: listDevicesQueryKey() })
  const one = useAction(signOutDeviceMutation())
  const others = useAction(signOutOtherDevicesMutation())
  return {
    // Undefined until they load.
    devices: computed<Device[] | undefined>(() => query.data.value?.devices),
    loadError: computed(() => (query.error.value ? describeError(query.error.value) : null)),
    actionError: computed(() => one.error.value ?? others.error.value),
    signOut: (deviceId: string) => {
      one.run({ path: { deviceId } }, refresh)
    },
    signOutOthers: () => {
      others.run({}, refresh)
    },
  }
}
