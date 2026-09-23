/**
 * Feedback for actions the user takes.
 *
 * A fire-and-forget `.mutate()` that reports nothing is indistinguishable from
 * a broken button — which is exactly how "Test key" and "Sync from provider"
 * read before this existed. Every mutation now says what happened, either here
 * or next to the field that caused it.
 */
import { toast } from 'sonner'

import { ApiError } from '@/lib/api'

declare module '@tanstack/react-query' {
  interface Register {
    mutationMeta: MutationMeta
  }
}

export interface MutationMeta extends Record<string, unknown> {
  /**
   * Set on mutations whose screen already renders the failure inline, so the
   * global handler does not report the same thing twice.
   */
  silentError?: boolean
}

/** Mutations that show their own error, spelled once so the intent is greppable. */
export const SILENT_ERROR: MutationMeta = { silentError: true }

const FALLBACK = 'Something went wrong. Please try again.'

/** Turns anything thrown by `apiFetch` into a sentence worth showing a user. */
export function errorMessage(error: unknown, fallback: string = FALLBACK): string {
  if (error instanceof ApiError) {
    // A validation failure names the field it rejected, which is more use than
    // the generic envelope message wrapping it.
    const field = Object.values(error.fields)[0]
    return field ?? error.message ?? fallback
  }
  if (error instanceof Error && error.message) return error.message
  return fallback
}

export { toast }
