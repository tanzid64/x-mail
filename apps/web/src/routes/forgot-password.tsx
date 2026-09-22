import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Link } from '@tanstack/react-router'
import { z } from 'zod'
import { Loader2, MailCheck } from 'lucide-react'

import { AuthShell } from '@/components/auth/auth-shell'
import { FieldError, FormError } from '@/components/auth/field-error'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useForgotPassword } from '@/features/auth/api'
import { applyApiErrors } from '@/lib/form'

const schema = z.object({
  email: z.string().min(1, 'Enter your email address.').email('Enter a valid email address.'),
})

type ForgotValues = z.infer<typeof schema>

export function ForgotPasswordPage() {
  const forgot = useForgotPassword()
  const [formError, setFormError] = useState<string | null>(null)
  const [sent, setSent] = useState(false)

  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<ForgotValues>({ resolver: zodResolver(schema), defaultValues: { email: '' } })

  const onSubmit = handleSubmit(async (values) => {
    setFormError(null)
    try {
      await forgot.mutateAsync(values)
      // The API deliberately answers the same way for unknown addresses, so the
      // UI must not imply the account exists either.
      setSent(true)
    } catch (error) {
      setFormError(applyApiErrors(error, setError))
    }
  })

  const backToSignIn = (
    <Link to="/login" search={{}} className="font-medium text-foreground underline-offset-4 hover:underline">
      Back to sign in
    </Link>
  )

  if (sent) {
    return (
      <AuthShell title="Check your email" footer={backToSignIn}>
        <div className="flex flex-col items-center gap-3 text-center">
          <span className="flex size-10 items-center justify-center rounded-full bg-muted">
            <MailCheck className="size-5 text-muted-foreground" />
          </span>
          <p className="text-sm text-muted-foreground">
            If that email is registered, a reset link is on its way. The link expires in one hour.
          </p>
        </div>
      </AuthShell>
    )
  }

  return (
    <AuthShell
      title="Reset your password"
      description="We'll email you a link to choose a new one."
      footer={backToSignIn}
    >
      <form onSubmit={onSubmit} className="flex flex-col gap-4" noValidate>
        <FormError message={formError} />

        <div className="flex flex-col gap-2">
          <Label htmlFor="email">Email</Label>
          <Input
            id="email"
            type="email"
            autoComplete="email"
            autoFocus
            placeholder="you@example.com"
            aria-invalid={Boolean(errors.email)}
            {...register('email')}
          />
          <FieldError message={errors.email?.message} />
        </div>

        <Button type="submit" disabled={isSubmitting} className="w-full">
          {isSubmitting ? <Loader2 className="size-4 animate-spin" /> : null}
          Send reset link
        </Button>
      </form>
    </AuthShell>
  )
}
