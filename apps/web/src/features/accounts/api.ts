import { useMutation, useQuery, useQueryClient, type UseQueryResult } from '@tanstack/react-query'

import { ApiError, apiFetch } from '@/lib/api'
import { SILENT_ERROR } from '@/lib/toast'
import type { Domain, MailAccount, MailAddress } from './types'

export const accountKeys = {
  all: ['mail-accounts'] as const,
  list: () => [...accountKeys.all, 'list'] as const,
  detail: (id: string) => [...accountKeys.all, 'detail', id] as const,
  domains: (accountId?: string) => ['domains', accountId ?? 'all'] as const,
  addresses: (accountId?: string) => ['addresses', accountId ?? 'all'] as const,
}

export function useMailAccounts(): UseQueryResult<MailAccount[], ApiError> {
  return useQuery<MailAccount[], ApiError>({
    queryKey: accountKeys.list(),
    queryFn: async () =>
      (await apiFetch<{ mail_accounts: MailAccount[] }>('/mail-accounts')).mail_accounts,
  })
}

export function useMailAccount(id: string): UseQueryResult<MailAccount, ApiError> {
  return useQuery<MailAccount, ApiError>({
    queryKey: accountKeys.detail(id),
    queryFn: async () =>
      (await apiFetch<{ mail_account: MailAccount }>(`/mail-accounts/${id}`)).mail_account,
  })
}

export function useDomains(accountId?: string): UseQueryResult<Domain[], ApiError> {
  return useQuery<Domain[], ApiError>({
    queryKey: accountKeys.domains(accountId),
    queryFn: async () => {
      const query = accountId ? `?mail_account_id=${accountId}` : ''
      return (await apiFetch<{ domains: Domain[] }>(`/domains${query}`)).domains
    },
  })
}

export function useAddresses(accountId?: string): UseQueryResult<MailAddress[], ApiError> {
  return useQuery<MailAddress[], ApiError>({
    queryKey: accountKeys.addresses(accountId),
    queryFn: async () => {
      const query = accountId ? `?mail_account_id=${accountId}` : ''
      return (await apiFetch<{ addresses: MailAddress[] }>(`/addresses${query}`)).addresses
    },
  })
}

/** Invalidates every account-derived query after a mutation changes the graph. */
function useInvalidateAccounts() {
  const queryClient = useQueryClient()
  return () => {
    void queryClient.invalidateQueries({ queryKey: accountKeys.all })
    void queryClient.invalidateQueries({ queryKey: ['domains'] })
    void queryClient.invalidateQueries({ queryKey: ['addresses'] })
  }
}

export function useConnectAccount() {
  const invalidate = useInvalidateAccounts()
  return useMutation<MailAccount, ApiError, { name: string; provider: string; api_key: string; color: string }>({
    mutationFn: async (input) =>
      (await apiFetch<{ mail_account: MailAccount }>('/mail-accounts', { method: 'POST', body: input }))
        .mail_account,
    meta: SILENT_ERROR,
    onSuccess: invalidate,
  })
}

export function useUpdateAccount(id: string) {
  const invalidate = useInvalidateAccounts()
  return useMutation<MailAccount, ApiError, { name?: string; color?: string; is_active?: boolean }>({
    mutationFn: async (input) =>
      (await apiFetch<{ mail_account: MailAccount }>(`/mail-accounts/${id}`, { method: 'PATCH', body: input }))
        .mail_account,
    onSuccess: invalidate,
  })
}

export function useDeleteAccount() {
  const invalidate = useInvalidateAccounts()
  return useMutation<void, ApiError, string>({
    mutationFn: (id) => apiFetch<void>(`/mail-accounts/${id}`, { method: 'DELETE' }),
    onSuccess: invalidate,
  })
}

export function useSetDefaultAccount() {
  const invalidate = useInvalidateAccounts()
  return useMutation<MailAccount[], ApiError, string>({
    mutationFn: async (id) =>
      (await apiFetch<{ mail_accounts: MailAccount[] }>(`/mail-accounts/${id}/default`, { method: 'POST' }))
        .mail_accounts,
    onSuccess: invalidate,
  })
}

export function useRotateAPIKey(id: string) {
  const invalidate = useInvalidateAccounts()
  return useMutation<MailAccount, ApiError, { api_key: string }>({
    mutationFn: async (input) =>
      (await apiFetch<{ mail_account: MailAccount }>(`/mail-accounts/${id}/api-key`, { method: 'POST', body: input }))
        .mail_account,
    meta: SILENT_ERROR,
    onSuccess: invalidate,
  })
}

export function useSetWebhookSecret(id: string) {
  const invalidate = useInvalidateAccounts()
  return useMutation<MailAccount, ApiError, { secret: string }>({
    mutationFn: async (input) =>
      (
        await apiFetch<{ mail_account: MailAccount }>(`/mail-accounts/${id}/webhook-secret`, {
          method: 'POST',
          body: input,
        })
      ).mail_account,
    meta: SILENT_ERROR,
    onSuccess: invalidate,
  })
}

export function useCheckCredentials(id: string) {
  const invalidate = useInvalidateAccounts()
  return useMutation<MailAccount, ApiError, void>({
    mutationFn: async () =>
      (await apiFetch<{ mail_account: MailAccount }>(`/mail-accounts/${id}/check`, { method: 'POST' }))
        .mail_account,
    onSuccess: invalidate,
  })
}

export function useSyncDomains(id: string) {
  const invalidate = useInvalidateAccounts()
  return useMutation<Domain[], ApiError, void>({
    mutationFn: async () =>
      (await apiFetch<{ domains: Domain[] }>(`/mail-accounts/${id}/sync-domains`, { method: 'POST' })).domains,
    onSuccess: invalidate,
  })
}

export function useRefreshDomain() {
  const invalidate = useInvalidateAccounts()
  return useMutation<Domain, ApiError, string>({
    mutationFn: async (domainId) =>
      (await apiFetch<{ domain: Domain }>(`/domains/${domainId}/refresh`, { method: 'POST' })).domain,
    onSuccess: invalidate,
  })
}

export function useCreateAddress(domainId: string) {
  const invalidate = useInvalidateAccounts()
  return useMutation<MailAddress, ApiError, { email: string; display_name: string }>({
    mutationFn: async (input) =>
      (await apiFetch<{ address: MailAddress }>(`/domains/${domainId}/addresses`, { method: 'POST', body: input }))
        .address,
    meta: SILENT_ERROR,
    onSuccess: invalidate,
  })
}

export function useDeleteAddress() {
  const invalidate = useInvalidateAccounts()
  return useMutation<void, ApiError, string>({
    mutationFn: (addressId) => apiFetch<void>(`/addresses/${addressId}`, { method: 'DELETE' }),
    onSuccess: invalidate,
  })
}
