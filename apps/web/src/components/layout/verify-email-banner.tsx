import { useState } from 'react'
import { MailWarning } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { useResendVerification } from '@/features/auth/api'
import type { User } from '@/features/auth/types'

interface VerifyEmailBannerProps {
  user: User
}

/** Prompts an unverified user to confirm their address. */
export function VerifyEmailBanner({ user }: VerifyEmailBannerProps) {
  const resend = useResendVerification()
  const [sent, setSent] = useState(false)

  if (user.email_verified_at) return null

  return (
    <div className="flex flex-wrap items-center gap-3 border-b bg-amber-50 px-4 py-2 text-sm text-amber-900 dark:bg-amber-950/40 dark:text-amber-200">
      <MailWarning className="size-4 shrink-0" />
      <span className="flex-1">
        Confirm <span className="font-medium">{user.email}</span> to secure your account.
      </span>
      {sent ? (
        <span className="text-xs">Confirmation email sent.</span>
      ) : (
        <Button
          size="sm"
          variant="outline"
          disabled={resend.isPending}
          onClick={() => {
            resend.mutate(undefined, { onSuccess: () => setSent(true) })
          }}
        >
          Resend
        </Button>
      )}
    </div>
  )
}
