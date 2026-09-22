import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Link, useNavigate } from '@tanstack/react-router'
import { z } from 'zod'
import { Loader2 } from 'lucide-react'

import { AuthShell } from '@/components/auth/auth-shell'
import { FieldError, FormError } from '@/components/auth/field-error'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useRegister } from '@/features/auth/api'
import { INBOX } from '@/router'
import { applyApiErrors } from '@/lib/form'

// Mirrors the server's rules so the obvious mistakes are caught without a round
// trip; the server remains the authority and its messages replace these.
const schema = z.object({
  name: z.string().max(120, 'Must be at most 120 characters.'),
  email: z.string().min(1, 'Enter your email address.').email('Enter a valid email address.'),
  password: z.string().min(10, 'Must be at least 10 characters.'),
})

type RegisterValues = z.infer<typeof schema>

export function RegisterPage() {
  const navigate = useNavigate()
  const signUp = useRegister()
  const [formError, setFormError] = useState<string | null>(null)

  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<RegisterValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: '', email: '', password: '' },
  })

  const onSubmit = handleSubmit(async (values) => {
    setFormError(null)
    try {
      await signUp.mutateAsync(values)
      await navigate(INBOX)
    } catch (error) {
      setFormError(applyApiErrors(error, setError))
    }
  })

  return (
    <AuthShell
      title="Create your account"
      description="One account, as many mail accounts as you need."
      footer={
        <>
          Already have an account?{' '}
          <Link to="/login" search={{}} className="font-medium text-foreground underline-offset-4 hover:underline">
            Sign in
          </Link>
        </>
      }
    >
      <form onSubmit={onSubmit} className="flex flex-col gap-4" noValidate>
        <FormError message={formError} />

        <div className="flex flex-col gap-2">
          <Label htmlFor="name">Name</Label>
          <Input id="name" autoComplete="name" autoFocus placeholder="Your name" {...register('name')} />
          <FieldError message={errors.name?.message} />
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="email">Email</Label>
          <Input
            id="email"
            type="email"
            autoComplete="email"
            placeholder="you@example.com"
            aria-invalid={Boolean(errors.email)}
            {...register('email')}
          />
          <FieldError message={errors.email?.message} />
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="password">Password</Label>
          <Input
            id="password"
            type="password"
            autoComplete="new-password"
            aria-invalid={Boolean(errors.password)}
            {...register('password')}
          />
          <FieldError message={errors.password?.message} />
          <p className="text-xs text-muted-foreground">At least 10 characters.</p>
        </div>

        <Button type="submit" disabled={isSubmitting} className="w-full">
          {isSubmitting ? <Loader2 className="size-4 animate-spin" /> : null}
          Create account
        </Button>
      </form>
    </AuthShell>
  )
}
