import { useState } from 'react'
import { Check, Copy, Loader2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useSetWebhookSecret } from '@/features/accounts/api'
import type { MailAccount } from '@/features/accounts/types'
import { ApiError } from '@/lib/api'

interface WebhookSectionProps {
  account: MailAccount
}

/**
 * Where the user wires their provider's callbacks to this account.
 *
 * The URL is per-account because each provider account signs with its own
 * secret; knowing which account a callback claims to be for is what lets the
 * server pick the one secret to verify against.
 */
export function WebhookSection({ account }: WebhookSectionProps) {
  const setSecret = useSetWebhookSecret(account.id)
  const [secret, setSecretValue] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)
  const [copied, setCopied] = useState(false)

  const url = `${window.location.origin}/api/v1/webhooks/resend/${account.id}`

  const onSave = async (event: React.FormEvent) => {
    event.preventDefault()
    setError(null)
    setSaved(false)
    try {
      await setSecret.mutateAsync({ secret })
      setSecretValue('')
      setSaved(true)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not save the signing secret.')
    }
  }

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(url)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    } catch {
      // Clipboard access can be refused; the URL is selectable either way.
    }
  }

  return (
    <section className="flex flex-col gap-3 rounded-lg border p-4">
      <h3 className="font-medium">Receiving mail</h3>
      <p className="text-sm text-muted-foreground">
        Add this URL as a webhook in Resend, then paste its signing secret below. Incoming mail is
        rejected unless it carries a valid signature.
      </p>

      <div className="flex items-center gap-2">
        <code className="min-w-0 flex-1 truncate rounded bg-muted px-2 py-1.5 font-mono text-xs" title={url}>
          {url}
        </code>
        <Button type="button" variant="outline" size="sm" className="gap-2 shrink-0" onClick={() => void copy()}>
          {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
          {copied ? 'Copied' : 'Copy'}
        </Button>
      </div>

      {account.webhook_secret_hint ? (
        <p className="text-sm text-muted-foreground">
          Current secret: <code className="rounded bg-muted px-1.5 py-0.5 font-mono">{account.webhook_secret_hint}</code>
        </p>
      ) : (
        <p className="text-sm text-amber-700 dark:text-amber-500">
          No signing secret set for this account — it falls back to the server-wide one, if configured.
        </p>
      )}

      <form onSubmit={onSave} className="flex flex-wrap items-end gap-2">
        <div className="flex min-w-48 flex-1 flex-col gap-1.5">
          <Label htmlFor="webhook-secret">Signing secret</Label>
          <Input
            id="webhook-secret"
            type="password"
            autoComplete="off"
            placeholder="whsec_…"
            value={secret}
            onChange={(event) => setSecretValue(event.target.value)}
          />
        </div>
        <Button type="submit" variant="outline" disabled={setSecret.isPending || secret.length === 0}>
          {setSecret.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
          Save secret
        </Button>
      </form>

      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : null}
      {saved ? (
        <p className="flex items-center gap-1 text-sm text-emerald-600">
          <Check className="size-3.5" />
          Signing secret saved.
        </p>
      ) : null}
    </section>
  )
}
