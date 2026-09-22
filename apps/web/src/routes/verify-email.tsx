import { useEffect, useRef, useState } from 'react'
import { Link, useSearch } from '@tanstack/react-router'
import { CircleCheck, CircleX, Loader2 } from 'lucide-react'

import { AuthShell } from '@/components/auth/auth-shell'
import { useVerifyEmail } from '@/features/auth/api'
import { ApiError } from '@/lib/api'
import { INBOX } from '@/router'

type Status = 'verifying' | 'verified' | 'failed'

export function VerifyEmailPage() {
  const { token } = useSearch({ from: '/verify-email' })
  const verify = useVerifyEmail()
  const [status, setStatus] = useState<Status>(token ? 'verifying' : 'failed')
  const [message, setMessage] = useState('This confirmation link is missing its token.')

  // The token is single-use, so React's development double-effect must not
  // spend it twice — the second call would report a valid link as expired.
  const attempted = useRef(false)

  useEffect(() => {
    if (!token || attempted.current) return
    attempted.current = true

    verify
      .mutateAsync({ token })
      .then(() => setStatus('verified'))
      .catch((error: unknown) => {
        setStatus('failed')
        setMessage(
          error instanceof ApiError
            ? error.message
            : 'We could not confirm your email address. Please try again.',
        )
      })
  }, [token, verify])

  return (
    <AuthShell
      title={
        status === 'verifying' ? 'Confirming your email' : status === 'verified' ? 'Email confirmed' : 'Link not valid'
      }
      footer={
        <Link {...INBOX} className="font-medium text-foreground underline-offset-4 hover:underline">
          Go to your inbox
        </Link>
      }
    >
      <div className="flex flex-col items-center gap-3 text-center">
        {status === 'verifying' ? (
          <Loader2 className="size-6 animate-spin text-muted-foreground" />
        ) : status === 'verified' ? (
          <CircleCheck className="size-6 text-emerald-600" />
        ) : (
          <CircleX className="size-6 text-destructive" />
        )}

        <p className="text-sm text-muted-foreground">
          {status === 'verifying'
            ? 'One moment…'
            : status === 'verified'
              ? 'Your address is confirmed. You can start using x-mail.'
              : message}
        </p>
      </div>
    </AuthShell>
  )
}
