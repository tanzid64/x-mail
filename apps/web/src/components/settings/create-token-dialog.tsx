import { useState } from 'react'
import { Check, Copy, KeyRound, Loader2, TriangleAlert } from 'lucide-react'

import { FieldError, FormError } from '@/components/auth/field-error'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useMailAccounts } from '@/features/accounts/api'
import { useCreateApiToken } from '@/features/tokens/api'
import { TOKEN_LIFETIMES, type TokenLifetime } from '@/features/tokens/types'
import { ApiError } from '@/lib/api'
import { toast } from '@/lib/toast'

/** The sentinel for "every account", since a Select cannot hold null. */
const ALL_ACCOUNTS = 'all'

/**
 * Creating an API token, and the one and only sight of the secret.
 *
 * The dialog deliberately does not close on success: the secret exists nowhere
 * else, so dismissing it is the user's decision to make once they have copied
 * it, not something that happens to them.
 */
export function CreateTokenDialog() {
  const create = useCreateApiToken()
  const { data: accounts } = useMailAccounts()

  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [lifetime, setLifetime] = useState<TokenLifetime>('30d')
  const [account, setAccount] = useState<string>(ALL_ACCOUNTS)

  const [secret, setSecret] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [nameError, setNameError] = useState<string | null>(null)
  const [formError, setFormError] = useState<string | null>(null)

  /**
   * Closing from our own buttons must reset too.
   *
   * Radix only calls onOpenChange for a close it initiates — Escape, the
   * overlay, the X — so setting `open` to false directly would leave the
   * secret in state, and the next "Create token" would reopen on the old one.
   */
  const close = () => {
    setOpen(false)
    reset()
  }

  const reset = () => {
    setName('')
    setLifetime('30d')
    setAccount(ALL_ACCOUNTS)
    setSecret(null)
    setCopied(false)
    setNameError(null)
    setFormError(null)
  }

  const onSubmit = async (event: React.FormEvent) => {
    event.preventDefault()
    setNameError(null)
    setFormError(null)
    try {
      const created = await create.mutateAsync({
        name,
        expires_in: lifetime,
        mail_account_id: account === ALL_ACCOUNTS ? null : account,
      })
      setSecret(created.token)
    } catch (error) {
      if (error instanceof ApiError && error.fields.name) {
        setNameError(error.fields.name)
        return
      }
      setFormError(
        error instanceof ApiError
          ? (error.fields.mail_account_id ?? error.fields.expires_in ?? error.message)
          : 'Could not create the token.',
      )
    }
  }

  const copy = async () => {
    if (!secret) return
    try {
      await navigator.clipboard.writeText(secret)
      setCopied(true)
      toast.success('Token copied to the clipboard.')
    } catch {
      // Clipboard access can be refused; the field is selectable either way.
      setFormError('Could not reach the clipboard — select the token and copy it manually.')
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) reset()
      }}
    >
      <DialogTrigger asChild>
        <Button className="gap-2">
          <KeyRound className="size-4" />
          Create token
        </Button>
      </DialogTrigger>

      <DialogContent>
        {secret ? (
          <>
            <DialogHeader>
              <DialogTitle>Copy your token now</DialogTitle>
              <DialogDescription>
                This is the only time it will be shown. We store a hash of it, so we cannot show it
                to you again — if you lose it, create another.
              </DialogDescription>
            </DialogHeader>

            <div className="flex items-start gap-2 rounded-md border border-amber-500/40 bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-950/40 dark:text-amber-200">
              <TriangleAlert className="mt-0.5 size-4 shrink-0" />
              <span>Treat it like a password. Anyone holding it can act as you.</span>
            </div>

            <div className="flex items-center gap-2">
              <Input
                readOnly
                value={secret}
                aria-label="Your new API token"
                data-testid="token-secret"
                className="font-mono text-xs"
                onFocus={(event) => event.target.select()}
              />
              <Button type="button" variant="outline" className="shrink-0 gap-2" onClick={() => void copy()}>
                {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
                {copied ? 'Copied' : 'Copy'}
              </Button>
            </div>

            <DialogFooter>
              <Button onClick={close}>Done</Button>
            </DialogFooter>
          </>
        ) : (
          <form onSubmit={onSubmit} className="flex flex-col gap-4">
            <DialogHeader>
              <DialogTitle>Create an API token</DialogTitle>
              <DialogDescription>
                A token acts as you over the API, so every endpoint you can reach in the browser is
                reachable with it.
              </DialogDescription>
            </DialogHeader>

            <div className="flex flex-col gap-1.5">
              <Label htmlFor="token-name">Name</Label>
              <Input
                id="token-name"
                placeholder="CI deploy"
                autoComplete="off"
                value={name}
                aria-invalid={Boolean(nameError)}
                onChange={(event) => setName(event.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                So you can recognise it later and know what to revoke.
              </p>
              <FieldError message={nameError ?? undefined} />
            </div>

            <div className="flex flex-col gap-1.5">
              <Label htmlFor="token-expiry">Expires</Label>
              <Select value={lifetime} onValueChange={(value) => setLifetime(value as TokenLifetime)}>
                <SelectTrigger id="token-expiry">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {TOKEN_LIFETIMES.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">
                {TOKEN_LIFETIMES.find((option) => option.value === lifetime)?.detail}
              </p>
            </div>

            <div className="flex flex-col gap-1.5">
              <Label htmlFor="token-account">Access</Label>
              <Select value={account} onValueChange={setAccount}>
                <SelectTrigger id="token-account">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ALL_ACCOUNTS}>All mail accounts</SelectItem>
                  {(accounts ?? []).map((option) => (
                    <SelectItem key={option.id} value={option.id}>
                      {option.name} only
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">
                {account === ALL_ACCOUNTS
                  ? 'The token reaches every mail account you own, including ones you add later.'
                  : 'The token can only see and act on that one mailbox.'}
              </p>
            </div>

            <FormError message={formError} />

            <DialogFooter>
              <Button type="button" variant="ghost" onClick={close}>
                Cancel
              </Button>
              <Button type="submit" disabled={create.isPending} className="gap-2">
                {create.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
                Create token
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}
