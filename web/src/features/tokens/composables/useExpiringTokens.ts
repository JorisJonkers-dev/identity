import { useQuery } from '@tanstack/vue-query'
import { computed, type Ref } from 'vue'
import { listMyTokensOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { expiring } from '../expiry'

/** The signed-in User's tokens that end within seven days, read once they are signed in. */
export function useExpiringTokens(signedIn: Ref<boolean>) {
  const query = useQuery(computed(() => ({ ...listMyTokensOptions(), retry: false, enabled: signedIn.value })))
  return computed(() => expiring(query.data.value?.tokens ?? []))
}
