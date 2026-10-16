import { useQuery } from '@tanstack/vue-query'
import { computed } from 'vue'
import { getDashboardOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { describeError } from '../../../infrastructure/problem'

/** How things stand: identity's own counts over the last day, for an Admin. */
export function useDashboard() {
  const query = useQuery({ ...getDashboardOptions(), retry: false })
  return {
    dashboard: computed(() => query.data.value),
    isLoading: query.isPending,
    loadError: computed(() => (query.error.value ? describeError(query.error.value) : null)),
  }
}
