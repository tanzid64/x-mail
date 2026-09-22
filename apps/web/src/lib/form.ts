import type { FieldValues, Path, UseFormSetError } from 'react-hook-form'

import { ApiError } from '@/lib/api'

/**
 * Moves server-side validation messages onto the matching form fields, and
 * returns whatever could not be attributed to a field so the caller can show it
 * as a form-level message.
 *
 * The API names fields exactly as the request body does, which is why the form
 * schema and the request payload deliberately share field names.
 */
export function applyApiErrors<T extends FieldValues>(
  error: unknown,
  setError: UseFormSetError<T>,
): string | null {
  if (!(error instanceof ApiError)) {
    return 'Something went wrong. Please try again.'
  }

  const entries = Object.entries(error.fields)
  if (entries.length === 0) {
    return error.message
  }

  for (const [field, message] of entries) {
    setError(field as Path<T>, { type: 'server', message })
  }
  return null
}
