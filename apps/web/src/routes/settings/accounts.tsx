import { Link } from '@tanstack/react-router'
import { CircleAlert, Inbox } from 'lucide-react'

import { ConnectAccountDialog } from '@/components/settings/connect-account-dialog'
import { AccountStatusBadge } from '@/components/settings/status-badge'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { useMailAccounts } from '@/features/accounts/api'
import { accountDotClass } from '@/features/accounts/types'
import { cn } from '@/lib/utils'

export function SettingsAccountsPage() {
  const { data: accounts, isLoading, error } = useMailAccounts()

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">Mail accounts</h2>
          <p className="text-sm text-muted-foreground">
            Each account has its own provider credentials, domains and mailbox.
          </p>
        </div>
        <ConnectAccountDialog />
      </div>

      {isLoading ? (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-20 w-full" />
          <Skeleton className="h-20 w-full" />
        </div>
      ) : error ? (
        <div role="alert" className="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">
          {error.message}
        </div>
      ) : accounts && accounts.length > 0 ? (
        <ul className="flex flex-col gap-2">
          {accounts.map((account) => (
            <li key={account.id}>
              <Link
                to="/settings/accounts/$accountId"
                params={{ accountId: account.id }}
                className="flex items-center gap-3 rounded-lg border p-4 transition hover:bg-muted/50"
              >
                <span className={cn('size-3 shrink-0 rounded-full', accountDotClass(account.color))} />

                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">{account.name}</span>
                    {account.is_default ? <Badge variant="outline">Default</Badge> : null}
                    <AccountStatusBadge status={account.status} />
                  </div>
                  <p className="mt-0.5 truncate text-sm text-muted-foreground">
                    Resend · {account.api_key_hint}
                  </p>
                  {account.status_message ? (
                    <p className="mt-1 flex items-center gap-1 text-sm text-destructive">
                      <CircleAlert className="size-3.5" />
                      {account.status_message}
                    </p>
                  ) : null}
                </div>
              </Link>
            </li>
          ))}
        </ul>
      ) : (
        <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed p-10 text-center">
          <Inbox className="size-6 text-muted-foreground" />
          <p className="font-medium">No mail accounts yet</p>
          <p className="max-w-sm text-sm text-muted-foreground">
            Connect a Resend account to start sending and receiving. You can add more than one and
            switch between them.
          </p>
        </div>
      )}
    </div>
  )
}
