interface FieldErrorProps {
  message?: string
}

export function FieldError({ message }: FieldErrorProps) {
  if (!message) return null
  return <p className="text-sm text-destructive">{message}</p>
}

interface FormErrorProps {
  message: string | null
}

/** Form-level failure, for errors that do not belong to a single field. */
export function FormError({ message }: FormErrorProps) {
  if (!message) return null
  return (
    <div
      role="alert"
      className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive"
    >
      {message}
    </div>
  )
}
