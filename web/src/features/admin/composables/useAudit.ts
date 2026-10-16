import { useInfiniteQuery, useQuery } from '@tanstack/vue-query'
import { computed, type Ref } from 'vue'
import { getAccessOptions, listAuditEventsInfiniteOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { AuditKind, AuditPage } from '../../../infrastructure/api/types.gen'
import { describeError } from '../../../infrastructure/problem'

/** What the Audit page narrows the events to; an empty field narrows nothing. */
export interface AuditFilter {
  user: string
  client: string
  kind: AuditKind | ''
}

/** The Audit Events matching a filter, newest first, a page at a time, and who and what they can name. */
export function useAudit(filter: Ref<AuditFilter>) {
  const events = useInfiniteQuery(
    computed(() => ({
      ...listAuditEventsInfiniteOptions({
        query: {
          user: filter.value.user || undefined,
          client: filter.value.client || undefined,
          kind: filter.value.kind || undefined,
        },
      }),
      initialPageParam: undefined as string | undefined,
      getNextPageParam: (page: AuditPage) => page.next,
      retry: false,
    })),
  )
  // The people and apps to filter by; an Admin who reads the log may read these too.
  const access = useQuery({ ...getAccessOptions(), retry: false })
  return {
    events: computed(() => events.data.value?.pages.flatMap((p) => p.events)),
    isLoading: events.isPending,
    loadError: computed(() => (events.error.value ? describeError(events.error.value) : null)),
    hasOlder: events.hasNextPage,
    loadingOlder: events.isFetchingNextPage,
    older: () => void events.fetchNextPage(),
    users: computed(() => access.data.value?.users ?? []),
    clients: computed(() => access.data.value?.clients ?? []),
  }
}
