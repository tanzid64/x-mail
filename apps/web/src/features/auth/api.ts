import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryResult,
} from '@tanstack/react-query'

import { ApiError, apiFetch } from '@/lib/api'
import type { MessageResponse, User, UserResponse } from './types'

export const authKeys = {
  session: ['auth', 'session'] as const,
}

/**
 * The signed-in user, or null when there is no session.
 *
 * A 401 is a valid answer here rather than an error: it is how the API says
 * "nobody is signed in". Every other failure is surfaced so the UI can
 * distinguish "signed out" from "the server is down".
 *
 * Shared with the router so route guards and components read one cache entry.
 */
export const sessionQueryOptions = queryOptions<User | null, ApiError>({
  queryKey: authKeys.session,
  queryFn: async () => {
    try {
      const { user } = await apiFetch<UserResponse>('/me')
      return user
    } catch (error) {
      if (error instanceof ApiError && error.isUnauthorized) return null
      throw error
    }
  },
  staleTime: 60_000,
  retry: (failureCount, error) => !error.isUnauthorized && failureCount < 2,
})

export function useSession(): UseQueryResult<User | null, ApiError> {
  return useQuery(sessionQueryOptions)
}

export function useLogin() {
  const queryClient = useQueryClient()
  return useMutation<User, ApiError, { email: string; password: string }>({
    mutationFn: async (input) => (await apiFetch<UserResponse>('/auth/login', { method: 'POST', body: input })).user,
    onSuccess: (user) => {
      queryClient.setQueryData(authKeys.session, user)
    },
  })
}

export function useRegister() {
  const queryClient = useQueryClient()
  return useMutation<User, ApiError, { email: string; password: string; name: string }>({
    mutationFn: async (input) =>
      (await apiFetch<UserResponse>('/auth/register', { method: 'POST', body: input })).user,
    onSuccess: (user) => {
      queryClient.setQueryData(authKeys.session, user)
    },
  })
}

export function useLogout() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, void>({
    mutationFn: () => apiFetch<void>('/auth/logout', { method: 'POST' }),
    onSettled: () => {
      // Whether or not the request succeeded, this browser is done with the
      // session — drop every cached query so no mailbox data outlives it.
      queryClient.setQueryData(authKeys.session, null)
      void queryClient.clear()
    },
  })
}

export function useForgotPassword() {
  return useMutation<MessageResponse, ApiError, { email: string }>({
    mutationFn: (input) => apiFetch<MessageResponse>('/auth/forgot-password', { method: 'POST', body: input }),
  })
}

export function useResetPassword() {
  return useMutation<MessageResponse, ApiError, { token: string; password: string }>({
    mutationFn: (input) => apiFetch<MessageResponse>('/auth/reset-password', { method: 'POST', body: input }),
  })
}

export function useVerifyEmail() {
  const queryClient = useQueryClient()
  return useMutation<User, ApiError, { token: string }>({
    mutationFn: async (input) =>
      (await apiFetch<UserResponse>('/auth/verify-email', { method: 'POST', body: input })).user,
    onSuccess: (user) => {
      queryClient.setQueryData(authKeys.session, user)
    },
  })
}

export function useResendVerification() {
  return useMutation<MessageResponse, ApiError, void>({
    mutationFn: () => apiFetch<MessageResponse>('/me/resend-verification', { method: 'POST' }),
  })
}

export function useUpdateProfile() {
  const queryClient = useQueryClient()
  return useMutation<User, ApiError, { name?: string; avatar_url?: string | null }>({
    mutationFn: async (input) => (await apiFetch<UserResponse>('/me', { method: 'PATCH', body: input })).user,
    onSuccess: (user) => {
      queryClient.setQueryData(authKeys.session, user)
    },
  })
}
