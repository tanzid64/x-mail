import { useMutation, useQuery, useQueryClient, type UseQueryResult } from '@tanstack/react-query'

import { mailboxKeys } from '@/features/mailbox/api'
import { ApiError, apiFetch } from '@/lib/api'
import type { Attachment, ComposeAddress, Draft } from './types'

export const draftKeys = {
  all: ['drafts'] as const,
  list: (account: string) => [...draftKeys.all, 'list', account] as const,
  detail: (id: string) => [...draftKeys.all, 'detail', id] as const,
}

export function useDrafts(account: string): UseQueryResult<Draft[], ApiError> {
  return useQuery<Draft[], ApiError>({
    queryKey: draftKeys.list(account),
    queryFn: async () => {
      const query = account === 'all' ? '' : `?mail_account_id=${account}`
      return (await apiFetch<{ drafts: Draft[] }>(`/drafts${query}`)).drafts
    },
  })
}

export function useDraft(draftId: string | null): UseQueryResult<Draft, ApiError> {
  return useQuery<Draft, ApiError>({
    queryKey: draftKeys.detail(draftId ?? ''),
    enabled: draftId !== null,
    queryFn: async () => (await apiFetch<{ draft: Draft }>(`/drafts/${draftId}`)).draft,
  })
}

interface CreateDraftInput {
  mail_account_id: string
  from_address_id?: string | null
  thread_id?: string | null
  in_reply_to_id?: string | null
  to?: ComposeAddress[]
  cc?: ComposeAddress[]
  bcc?: ComposeAddress[]
  subject?: string
  html_body?: string
  text_body?: string
}

export function useCreateDraft() {
  const queryClient = useQueryClient()
  return useMutation<Draft, ApiError, CreateDraftInput>({
    mutationFn: async (input) =>
      (await apiFetch<{ draft: Draft }>('/drafts', { method: 'POST', body: input })).draft,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: draftKeys.all })
    },
  })
}

interface UpdateDraftInput {
  id: string
  from_address_id?: string | null
  to?: ComposeAddress[]
  cc?: ComposeAddress[]
  bcc?: ComposeAddress[]
  subject?: string
  html_body?: string
  text_body?: string
}

/**
 * Autosave.
 *
 * Deliberately does not invalidate the draft list on every save: it fires every
 * few seconds while typing, and refetching that often would fight the editor
 * for the user's connection. The list is refreshed when the window closes.
 */
export function useSaveDraft() {
  return useMutation<Draft, ApiError, UpdateDraftInput>({
    mutationFn: async ({ id, ...body }) =>
      (await apiFetch<{ draft: Draft }>(`/drafts/${id}`, { method: 'PATCH', body })).draft,
  })
}

export function useDeleteDraft() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: (id) => apiFetch<void>(`/drafts/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: draftKeys.all })
    },
  })
}

export function useSendDraft() {
  const queryClient = useQueryClient()
  return useMutation<{ message_id: string; thread_id: string }, ApiError, string>({
    mutationFn: (id) =>
      apiFetch<{ message_id: string; thread_id: string }>(`/drafts/${id}/send`, { method: 'POST' }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: draftKeys.all })
      // The message appears in Sent straight away, queued.
      void queryClient.invalidateQueries({ queryKey: mailboxKeys.all })
    },
  })
}

/** Uploads one file. Multipart, so it does not go through apiFetch. */
export function useUploadAttachment() {
  const queryClient = useQueryClient()

  return useMutation<Attachment, ApiError, { draftId: string; file: File }>({
    mutationFn: async ({ draftId, file }) => {
      const form = new FormData()
      form.append('file', file)

      const response = await fetch(`/api/v1/drafts/${draftId}/attachments`, {
        method: 'POST',
        credentials: 'include',
        body: form,
      })

      const text = await response.text()
      const payload = text ? (JSON.parse(text) as Record<string, unknown>) : {}

      if (!response.ok) {
        const body = payload.error as { code: string; message: string } | undefined
        throw new ApiError(response.status, body ?? { code: 'upload_failed', message: 'Upload failed.' })
      }
      return (payload as { attachment: Attachment }).attachment
    },
    onSuccess: (_attachment, { draftId }) => {
      void queryClient.invalidateQueries({ queryKey: draftKeys.detail(draftId) })
    },
  })
}

export function useDeleteAttachment() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, { attachmentId: string; draftId: string }>({
    mutationFn: ({ attachmentId }) => apiFetch<void>(`/attachments/${attachmentId}`, { method: 'DELETE' }),
    onSuccess: (_result, { draftId }) => {
      void queryClient.invalidateQueries({ queryKey: draftKeys.detail(draftId) })
    },
  })
}
