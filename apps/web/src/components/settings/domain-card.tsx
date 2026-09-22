import { useState } from 'react'
import { Loader2, Plus, RefreshCw, Trash2 } from 'lucide-react'

import { DomainStatusBadge } from '@/components/settings/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { useCreateAddress, useDeleteAddress, useRefreshDomain } from '@/features/accounts/api'
import type { Domain, MailAddress } from '@/features/accounts/types'
import { ApiError } from '@/lib/api'

interface DomainCardProps {
  domain: Domain
  addresses: MailAddress[]
}

export function DomainCard({ domain, addresses }: DomainCardProps) {
  const refresh = useRefreshDomain()
  const createAddress = useCreateAddress(domain.id)
  const deleteAddress = useDeleteAddress()

  const [email, setEmail] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [addressError, setAddressError] = useState<string | null>(null)

  const onAddAddress = async (event: React.FormEvent) => {
    event.preventDefault()
    setAddressError(null)
    try {
      await createAddress.mutateAsync({ email, display_name: displayName })
      setEmail('')
      setDisplayName('')
    } catch (error) {
      setAddressError(
        error instanceof ApiError
          ? (error.fields.email ?? error.message)
          : 'Could not add that address.',
      )
    }
  }

  return (
    // Named so assistive tech (and tests) can address one domain's card.
    <section aria-label={domain.domain} className="rounded-lg border">
      <header className="flex flex-wrap items-center gap-3 border-b p-4">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="font-medium">{domain.domain}</h3>
            <DomainStatusBadge status={domain.status} />
          </div>
          {domain.region ? (
            <p className="mt-0.5 text-sm text-muted-foreground">{domain.region}</p>
          ) : null}
        </div>

        <Button
          variant="outline"
          size="sm"
          className="gap-2"
          disabled={refresh.isPending}
          onClick={() => refresh.mutate(domain.id)}
        >
          {refresh.isPending ? (
            <Loader2 className="size-3.5 animate-spin" />
          ) : (
            <RefreshCw className="size-3.5" />
          )}
          Recheck DNS
        </Button>
      </header>

      {/* DNS records only matter while the domain is not yet usable. */}
      {domain.status !== 'verified' && domain.records.length > 0 ? (
        <div className="border-b p-4">
          <p className="mb-2 text-sm text-muted-foreground">
            Publish these records at your DNS provider, then recheck.
          </p>
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Type</TableHead>
                  <TableHead>Name</TableHead>
                  <TableHead>Value</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {domain.records.map((record, index) => (
                  <TableRow key={`${record.type}-${record.name}-${index}`}>
                    <TableCell className="font-mono text-xs">{record.type}</TableCell>
                    <TableCell className="font-mono text-xs">{record.name}</TableCell>
                    <TableCell className="max-w-xs truncate font-mono text-xs" title={record.value}>
                      {record.value}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </div>
      ) : null}

      <div className="p-4">
        <h4 className="mb-2 text-sm font-medium">Sender addresses</h4>

        {addresses.length > 0 ? (
          <ul className="mb-4 flex flex-col divide-y rounded-md border">
            {addresses.map((address) => (
              <li key={address.id} className="flex items-center gap-3 px-3 py-2">
                <div className="min-w-0 flex-1">
                  <span className="text-sm">{address.email}</span>
                  {address.display_name ? (
                    <span className="ml-2 text-sm text-muted-foreground">{address.display_name}</span>
                  ) : null}
                </div>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={`Remove ${address.email}`}
                  disabled={deleteAddress.isPending}
                  onClick={() => deleteAddress.mutate(address.id)}
                >
                  <Trash2 className="size-4" />
                </Button>
              </li>
            ))}
          </ul>
        ) : (
          <p className="mb-4 text-sm text-muted-foreground">No addresses on this domain yet.</p>
        )}

        <form onSubmit={onAddAddress} className="flex flex-wrap items-end gap-2">
          <div className="flex min-w-48 flex-1 flex-col gap-1.5">
            <Label htmlFor={`email-${domain.id}`} className="text-xs">
              Address
            </Label>
            <Input
              id={`email-${domain.id}`}
              placeholder={`hello@${domain.domain}`}
              value={email}
              onChange={(event) => setEmail(event.target.value)}
            />
          </div>
          <div className="flex min-w-40 flex-1 flex-col gap-1.5">
            <Label htmlFor={`name-${domain.id}`} className="text-xs">
              Display name
            </Label>
            <Input
              id={`name-${domain.id}`}
              placeholder="Support"
              value={displayName}
              onChange={(event) => setDisplayName(event.target.value)}
            />
          </div>
          <Button type="submit" variant="outline" className="gap-2" disabled={createAddress.isPending}>
            {createAddress.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Plus className="size-4" />
            )}
            Add
          </Button>
        </form>

        {addressError ? (
          <p role="alert" className="mt-2 text-sm text-destructive">
            {addressError}
          </p>
        ) : null}
      </div>
    </section>
  )
}
