import { useState } from 'react'
import { Link, useNavigate, useParams } from '@tanstack/react-router'
import { ArrowLeft, Check, Loader2, RefreshCw, ShieldCheck, Trash2 } from 'lucide-react'

import { DomainCard } from '@/components/settings/domain-card'
import { WebhookSection } from '@/components/settings/webhook-section'
import { AccountStatusBadge } from '@/components/settings/status-badge'
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
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import {
  useAddresses,
  useCheckCredentials,
  useDeleteAccount,
  useDomains,
  useMailAccount,
  useRotateAPIKey,
  useSetDefaultAccount,
  useSyncDomains,
  useUpdateAccount,
} from '@/features/accounts/api'
import { ACCOUNT_COLORS, accountDotClass } from '@/features/accounts/types'
import { ApiError } from '@/lib/api'
import { cn } from '@/lib/utils'
import { useUIStore } from '@/stores/ui'

export function SettingsAccountDetailPage() {
  const { accountId } = useParams({ from: '/_app/settings/accounts/$accountId' })
  const navigate = useNavigate()

  const { data: account, isLoading, error } = useMailAccount(accountId)
  const { data: domains } = useDomains(accountId)
  const { data: addresses } = useAddresses(accountId)

  const update = useUpdateAccount(accountId)
  const rotate = useRotateAPIKey(accountId)
  const check = useCheckCredentials(accountId)
  const sync = useSyncDomains(accountId)
  const setDefault = useSetDefaultAccount()
  const remove = useDeleteAccount()

  const activeAccount = useUIStore((state) => state.activeAccount)
  const setActiveAccount = useUIStore((state) => state.setActiveAccount)

  const [newKey, setNewKey] = useState('')
  const [keyError, setKeyError] = useState<string | null>(null)
  const [keyRotated, setKeyRotated] = useState(false)

  if (isLoading) {
    return <Skeleton className="h-64 w-full" />
  }

  if (error || !account) {
    return (
      <div className="flex flex-col items-start gap-3">
        <p className="text-sm text-muted-foreground">
          {error?.status === 404 ? 'That mail account does not exist.' : error?.message}
        </p>
        <Button asChild variant="outline">
          <Link to="/settings/accounts">Back to mail accounts</Link>
        </Button>
      </div>
    )
  }

  const onRotate = async (event: React.FormEvent) => {
    event.preventDefault()
    setKeyError(null)
    setKeyRotated(false)
    try {
      await rotate.mutateAsync({ api_key: newKey })
      setNewKey('')
      setKeyRotated(true)
    } catch (err) {
      setKeyError(
        err instanceof ApiError ? (err.fields.api_key ?? err.message) : 'Could not update the key.',
      )
    }
  }

  const onDelete = async () => {
    await remove.mutateAsync(accountId)
    // The deleted account must not stay selected in the sidebar.
    if (activeAccount === accountId) setActiveAccount('all')
    await navigate({ to: '/settings/accounts' })
  }

  const domainAddresses = (domainId: string) =>
    (addresses ?? []).filter((address) => address.domain_id === domainId)

  return (
    <div className="flex flex-col gap-6">
      <div>
        <Button asChild variant="ghost" size="sm" className="-ml-2 gap-2">
          <Link to="/settings/accounts">
            <ArrowLeft className="size-4" />
            Mail accounts
          </Link>
        </Button>
      </div>

      <header className="flex flex-wrap items-center gap-3">
        <span className={cn('size-3 rounded-full', accountDotClass(account.color))} />
        <h2 className="text-lg font-semibold">{account.name}</h2>
        <AccountStatusBadge status={account.status} />
        {account.is_default ? null : (
          <Button
            variant="outline"
            size="sm"
            disabled={setDefault.isPending}
            onClick={() => setDefault.mutate(accountId)}
          >
            Make default
          </Button>
        )}
      </header>

      {account.status_message ? (
        <div role="alert" className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {account.status_message}
        </div>
      ) : null}

      <section className="flex flex-col gap-3 rounded-lg border p-4">
        <h3 className="font-medium">Account</h3>

        <div className="flex flex-wrap items-end gap-3">
          <div className="flex min-w-48 flex-1 flex-col gap-1.5">
            <Label htmlFor="account-name">Name</Label>
            <Input
              id="account-name"
              defaultValue={account.name}
              onBlur={(event) => {
                const name = event.target.value.trim()
                if (name && name !== account.name) update.mutate({ name })
              }}
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <Label>Colour</Label>
            <div className="flex gap-2">
              {ACCOUNT_COLORS.map((color) => (
                <button
                  key={color}
                  type="button"
                  aria-label={color}
                  aria-pressed={account.color === color}
                  onClick={() => update.mutate({ color })}
                  className={cn(
                    'size-7 rounded-full ring-offset-2 transition',
                    accountDotClass(color),
                    account.color === color ? 'ring-2 ring-foreground' : 'opacity-60 hover:opacity-100',
                  )}
                />
              ))}
            </div>
          </div>
        </div>
      </section>

      <section className="flex flex-col gap-3 rounded-lg border p-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 className="font-medium">API key</h3>
          <Button
            variant="outline"
            size="sm"
            className="gap-2"
            disabled={check.isPending}
            onClick={() => check.mutate()}
          >
            {check.isPending ? (
              <Loader2 className="size-3.5 animate-spin" />
            ) : (
              <ShieldCheck className="size-3.5" />
            )}
            Test key
          </Button>
        </div>

        <p className="text-sm text-muted-foreground">
          Stored encrypted; only this masked form is ever sent to the browser.
        </p>
        <code className="w-fit rounded bg-muted px-2 py-1 font-mono text-sm">{account.api_key_hint}</code>

        <form onSubmit={onRotate} className="flex flex-wrap items-end gap-2">
          <div className="flex min-w-48 flex-1 flex-col gap-1.5">
            <Label htmlFor="new-key">Replace with a new key</Label>
            <Input
              id="new-key"
              type="password"
              autoComplete="off"
              placeholder="re_…"
              value={newKey}
              onChange={(event) => setNewKey(event.target.value)}
            />
          </div>
          <Button type="submit" variant="outline" disabled={rotate.isPending || newKey.length === 0}>
            {rotate.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
            Update key
          </Button>
        </form>

        {keyError ? (
          <p role="alert" className="text-sm text-destructive">
            {keyError}
          </p>
        ) : null}
        {keyRotated ? (
          <p className="flex items-center gap-1 text-sm text-emerald-600">
            <Check className="size-3.5" />
            Key updated.
          </p>
        ) : null}
      </section>

      <WebhookSection account={account} />

      <section className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 className="font-medium">Domains</h3>
          <Button
            variant="outline"
            size="sm"
            className="gap-2"
            disabled={sync.isPending}
            onClick={() => sync.mutate()}
          >
            {sync.isPending ? (
              <Loader2 className="size-3.5 animate-spin" />
            ) : (
              <RefreshCw className="size-3.5" />
            )}
            Sync from provider
          </Button>
        </div>

        {domains && domains.length > 0 ? (
          domains.map((domain) => (
            <DomainCard key={domain.id} domain={domain} addresses={domainAddresses(domain.id)} />
          ))
        ) : (
          <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
            No domains on this provider account yet. Add one in Resend, then sync.
          </div>
        )}
      </section>

      <section className="flex flex-col gap-3 rounded-lg border border-destructive/30 p-4">
        <h3 className="font-medium text-destructive">Remove this account</h3>
        <p className="text-sm text-muted-foreground">
          Removing it deletes its domains, addresses and stored mail. Your other accounts are not
          affected.
        </p>

        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button variant="destructive" className="w-fit gap-2">
              <Trash2 className="size-4" />
              Remove account
            </Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Remove “{account.name}”?</AlertDialogTitle>
              <AlertDialogDescription>
                This deletes the account's domains, addresses and stored mail. It cannot be undone.
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancel</AlertDialogCancel>
              <AlertDialogAction onClick={() => void onDelete()}>Remove</AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </section>
    </div>
  )
}
