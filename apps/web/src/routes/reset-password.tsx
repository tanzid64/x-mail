import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { z } from 'zod'
import { Loader2 } from 'lucide-react'

import { AuthShell } from '@/components/auth/auth-shell'
import { FieldError, FormError } from '@/components/auth/field-error'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useResetPassword } from '@/features/auth/api'
import { applyApiErrors } from '@/lib/form'

const schema = z
  .object({
    password: z.string().min(10, 'Must be at least 10 characters.'),
    confirm: z.string(),
  })
  .refine((values) => values.password === values.confirm, {
    path: ['confirm'],
    message: 'Both passwords must match.',
  })

type ResetValues = z.infer<typeof schema>

export function ResetPasswordPage() {
  const navigate = useNavigate()
  const { token } = useSearch({ from: '/reset-password' })
  const reset = useResetPassword()
  const [formError, setFormError] = useState<string | null>(null)

  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<ResetValues>({ resolver: zodResolver(schema), defaultValues: { password: '', confirm: '' } })

  const onSubmit = handleSubmit(async (values) => {
    setFormError(null)
    try {
      await reset.mutateAsync({ token: token ?? '', password: values.password })
      await navigate({ to: '/login', search: {} })
    } catch (error) {
      setFormError(applyApiErrors(error, setError))
    }
  })

  if (!token) {
    return (
      <AuthShell title="Link is incomplete">
        <p className="text-center text-sm text-muted-foreground">
          This reset link is missing its token. Request a new one from the{' '}
          <Link to="/forgot-password" className="font-medium text-foreground underline-offset-4 hover:underline">
            forgot password
          </Link>{' '}
          page.
        </p>
      </AuthShell>
    )
  }

  return (
    <AuthShell title="Choose a new password" description="You'll be signed out everywhere once it changes.">
      <form onSubmit={onSubmit} className="flex flex-col gap-4" noValidate>
        <FormError message={formError} />

        <div className="flex flex-col gap-2">
          <Label htmlFor="password">New password</Label>
          <Input
            id="password"
            type="password"
            autoComplete="new-password"
            autoFocus
            aria-invalid={Boolean(errors.password)}
            {...register('password')}
          />
          <FieldError message={errors.password?.message} />
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="confirm">Confirm new password</Label>
          <Input
            id="confirm"
            type="password"
            autoComplete="new-password"
            aria-invalid={Boolean(errors.confirm)}
            {...register('confirm')}
          />
          <FieldError message={errors.confirm?.message} />
        </div>

        <Button type="submit" disabled={isSubmitting} className="w-full">
          {isSubmitting ? <Loader2 className="size-4 animate-spin" /> : null}
          Change password
        </Button>
      </form>
    </AuthShell>
  )
}
