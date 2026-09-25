import { Link } from '@tanstack/react-router'
import { BookOpen, CircleAlert, KeyRound, Trash2 } from 'lucide-react'

import { CreateTokenDialog } from '@/components/settings/create-token-dialog'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useMailAccounts } from '@/features/accounts/api'
import { useApiTokens, useRevokeApiToken } from '@/features/tokens/api'
import type { ApiToken } from '@/features/tokens/types'
import { formatFullDate } from '@/lib/format'
import { toast } from '@/lib/toast'

export function SettingsApiTokensPage() {
  const { data: tokens, isLoading, error, dataUpdatedAt } = useApiTokens()
  const { data: accounts } = useMailAccounts()
  const revoke = useRevokeApiToken()

  const accountName = (id: string | null) =>
    id ? ((accounts ?? []).find((account) => account.id === id)?.name ?? 'one mail account') : null

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">API tokens</h2>
          <p className="text-sm text-muted-foreground">
            Use a token to reach the API from your own code, with{' '}
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
              Authorization: Bearer …
            </code>
            .{' '}
            <a
              href="/api/v1/docs"
              target="_blank"
              rel="noreferrer"
              className="underline underline-offset-4 hover:text-foreground"
            >
              Try endpoints interactively
            </a>
            , or{' '}
            <Link
              to="/docs/$page"
              params={{ page: 'api-mcp' }}
              className="underline underline-offset-4 hover:text-foreground"
            >
              connect an AI assistant over MCP
            </Link>
            .
          </p>
        </div>
        <div className="flex items-center gap-2">
          {/* The written guide first: Swagger is for poking at endpoints, not
              for learning what they mean. */}
          <Button asChild variant="outline" className="gap-2">
            <Link to="/docs/$page" params={{ page: 'api' }}>
              <BookOpen className="size-4" />
              API reference
            </Link>
          </Button>
          <CreateTokenDialog />
        </div>
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
      ) : tokens && tokens.length > 0 ? (
        <ul data-testid="api-token-list" className="flex flex-col gap-2">
          {tokens.map((token) => (
            <li key={token.id} className="flex flex-wrap items-center gap-3 rounded-lg border p-4">
              <KeyRound className="size-4 shrink-0 text-muted-foreground" />

              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium">{token.name}</span>
                  <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">{token.hint}</code>
                  <ExpiryBadge token={token} now={dataUpdatedAt} />
                  {token.mail_account_id ? (
                    <Badge variant="outline">{accountName(token.mail_account_id)} only</Badge>
                  ) : null}
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  Created {formatFullDate(token.created_at)} ·{' '}
                  {token.last_used_at ? `last used ${formatFullDate(token.last_used_at)}` : 'never used'}
                </p>
              </div>

              <AlertDialog>
                <AlertDialogTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={`Revoke ${token.name}`}
                    disabled={revoke.isPending}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>Revoke “{token.name}”?</AlertDialogTitle>
                    <AlertDialogDescription>
                      Anything using this token stops working immediately. This cannot be undone.
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>Cancel</AlertDialogCancel>
                    <AlertDialogAction
                      onClick={() =>
                        revoke.mutate(token.id, {
                          onSuccess: () => toast.success(`“${token.name}” was revoked.`),
                        })
                      }
                    >
                      Revoke
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </li>
          ))}
        </ul>
      ) : (
        <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed p-10 text-center">
          <KeyRound className="size-8 text-muted-foreground" />
          <div>
            <p className="font-medium">No API tokens yet</p>
            <p className="text-sm text-muted-foreground">
              Create one to let a script or another service act on your mail.
            </p>
          </div>
        </div>
      )}
    </div>
  )
}

/**
 * How long is left, rather than a raw date.
 *
 * "Expires in 3 days" is the thing worth acting on; the exact instant is not.
 * A token already past its date is called out in red, because it is still
 * listed but will refuse every request.
 *
 * `now` is when the list was fetched, not `Date.now()`. Reading the clock
 * during render is impure, and pinning it at mount instead would measure a
 * token created minutes later against a stale instant — which rounded a
 * fresh 30-day token up to "31 days".
 */
function ExpiryBadge({ token, now }: { token: ApiToken; now: number }) {
  if (!token.expires_at) {
    return <Badge variant="secondary">Never expires</Badge>
  }

  const remainingMs = new Date(token.expires_at).getTime() - now
  if (remainingMs <= 0) {
    return (
      <Badge variant="destructive" className="gap-1">
        <CircleAlert className="size-3" />
        Expired
      </Badge>
    )
  }

  const days = Math.ceil(remainingMs / (24 * 60 * 60 * 1000))
  return (
    <Badge variant={days <= 7 ? 'destructive' : 'outline'}>
      {days === 1 ? 'Expires tomorrow' : `Expires in ${days} days`}
    </Badge>
  )
}
