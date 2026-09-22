import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryResult,
} from '@tanstack/react-query'

import { ApiError, apiFetch } from '@/lib/api'
import type { Label, ThreadAction, ThreadDetail, ThreadPage } from './types'

export const mailboxKeys = {
  all: ['mailbox'] as const,
  threads: (folder: string, account: string, labelId?: string) =>
    [...mailboxKeys.all, 'threads', folder, account, labelId ?? 'none'] as const,
  thread: (id: string) => [...mailboxKeys.all, 'thread', id] as const,
  counts: (account: string) => [...mailboxKeys.all, 'counts', account] as const,
  labels: (account: string) => [...mailboxKeys.all, 'labels', account] as const,
}

interface ThreadListOptions {
  folder: string
  /** "all" for the unified view, or a mail account id. */
  account: string
  labelId?: string
}

function accountQuery(account: string): string {
  return account === 'all' ? '' : `&mail_account_id=${account}`
}

/**
 * Infinite list of threads. The API returns an opaque cursor rather than an
 * offset, because a mailbox changes under the reader and an offset would skip
 * or repeat rows as mail arrives.
 */
export function useThreads({ folder, account, labelId }: ThreadListOptions) {
  return useInfiniteQuery<ThreadPage, ApiError>({
    queryKey: mailboxKeys.threads(folder, account, labelId),
    initialPageParam: '',
    queryFn: ({ pageParam }) => {
      const cursor = pageParam ? `&cursor=${encodeURIComponent(String(pageParam))}` : ''
      const label = labelId ? `&label_id=${labelId}` : ''
      return apiFetch<ThreadPage>(`/threads?folder=${folder}${accountQuery(account)}${label}${cursor}`)
    },
    getNextPageParam: (lastPage) => lastPage.next_cursor || undefined,
  })
}

export function useThread(threadId: string, markRead = true): UseQueryResult<ThreadDetail, ApiError> {
  return useQuery<ThreadDetail, ApiError>({
    queryKey: mailboxKeys.thread(threadId),
    queryFn: () => apiFetch<ThreadDetail>(`/threads/${threadId}?mark_read=${markRead}`),
  })
}

export function useFolderCounts(account: string): UseQueryResult<Record<string, number>, ApiError> {
  return useQuery<Record<string, number>, ApiError>({
    queryKey: mailboxKeys.counts(account),
    queryFn: async () => {
      const query = account === 'all' ? '' : `?mail_account_id=${account}`
      return (await apiFetch<{ counts: Record<string, number> }>(`/threads/counts${query}`)).counts
    },
  })
}

export function useLabels(account: string): UseQueryResult<Label[], ApiError> {
  return useQuery<Label[], ApiError>({
    queryKey: mailboxKeys.labels(account),
    queryFn: async () => {
      const query = account === 'all' ? '' : `?mail_account_id=${account}`
      return (await apiFetch<{ labels: Label[] }>(`/labels${query}`)).labels
    },
  })
}

interface ActionInput {
  action: ThreadAction | 'label' | 'unlabel'
  threadIds: string[]
  labelId?: string
}

/**
 * Applies a bulk action, then invalidates the mailbox.
 *
 * Invalidation rather than hand-patching the cache is deliberate: an action can
 * move a thread out of the current folder, change its unread count and change
 * the sidebar badge at once, and reproducing all of that client-side is how
 * lists drift out of sync with the server.
 */
export function useThreadAction() {
  const queryClient = useQueryClient()

  return useMutation<{ affected: number }, ApiError, ActionInput>({
    mutationFn: ({ action, threadIds, labelId }) =>
      apiFetch<{ affected: number }>('/threads/actions', {
        method: 'POST',
        body: { action, thread_ids: threadIds, label_id: labelId },
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: mailboxKeys.all })
    },
  })
}
