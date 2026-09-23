import { useMutation, useQuery, useQueryClient, type UseQueryResult } from '@tanstack/react-query'

import { ApiError, apiFetch } from '@/lib/api'
import { SILENT_ERROR } from '@/lib/toast'
import type { ApiToken, CreatedApiToken, TokenLifetime } from './types'

export const tokenKeys = {
  all: ['api-tokens'] as const,
  list: () => [...tokenKeys.all, 'list'] as const,
}

export function useApiTokens(): UseQueryResult<ApiToken[], ApiError> {
  return useQuery<ApiToken[], ApiError>({
    queryKey: tokenKeys.list(),
    queryFn: async () => (await apiFetch<{ api_tokens: ApiToken[] }>('/api-tokens')).api_tokens,
  })
}

export interface CreateTokenInput {
  name: string
  expires_in: TokenLifetime
  mail_account_id?: string | null
}

export function useCreateApiToken() {
  const queryClient = useQueryClient()
  return useMutation<CreatedApiToken, ApiError, CreateTokenInput>({
    mutationFn: (input) => apiFetch<CreatedApiToken>('/api-tokens', { method: 'POST', body: input }),
    // The dialog reports its own failure next to the field that caused it.
    meta: SILENT_ERROR,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: tokenKeys.all })
    },
  })
}

export function useRevokeApiToken() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: (id) => apiFetch<void>(`/api-tokens/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: tokenKeys.all })
    },
  })
}
