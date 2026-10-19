import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { computed } from 'vue'
import {
  changePasswordMutation,
  getProfileOptions,
  getProfileQueryKey,
  updateProfileMutation,
} from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { Profile } from '../../../infrastructure/api/types.gen'
import { describeError } from '../../../infrastructure/problem'
import { useAction } from '../../../infrastructure/useAction'

/** The signed-in User's Profile, and what they can change about it. */
export function useProfile() {
  const queryClient = useQueryClient()
  const query = useQuery({ ...getProfileOptions(), retry: false })
  const rename = useAction(updateProfileMutation())
  return {
    profile: computed<Profile | undefined>(() => query.data.value),
    loadError: computed(() => (query.error.value ? describeError(query.error.value) : null)),
    rename: {
      ...rename,
      run: (name: string) => {
        rename.run({ body: { name } }, () => void queryClient.invalidateQueries({ queryKey: getProfileQueryKey() }))
      },
    },
    changePassword: useAction(changePasswordMutation()),
  }
}
