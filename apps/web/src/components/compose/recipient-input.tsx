import { useState, type KeyboardEvent } from 'react'
import { X } from 'lucide-react'

import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { ComposeAddress } from '@/features/compose/types'
import { cn } from '@/lib/utils'

interface RecipientInputProps {
  id: string
  label: string
  value: ComposeAddress[]
  onChange: (value: ComposeAddress[]) => void
}

/** Rough shape check; the server is the authority on what is deliverable. */
const LOOKS_LIKE_EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

/**
 * Recipient chips. Typing an address and pressing Enter, Tab or comma commits
 * it; Backspace on an empty field removes the last one, which is how every mail
 * client behaves.
 */
export function RecipientInput({ id, label, value, onChange }: RecipientInputProps) {
  const [draft, setDraft] = useState('')
  const [error, setError] = useState<string | null>(null)

  const commit = (raw: string): boolean => {
    const email = raw.trim().replace(/[,;]$/, '').toLowerCase()
    if (email === '') return true

    if (!LOOKS_LIKE_EMAIL.test(email)) {
      setError(`“${email}” is not a valid email address.`)
      return false
    }
    if (value.some((address) => address.email === email)) {
      setDraft('')
      return true
    }
    onChange([...value, { email }])
    setDraft('')
    setError(null)
    return true
  }

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter' || event.key === 'Tab' || event.key === ',' || event.key === ';') {
      if (draft.trim() === '') return
      event.preventDefault()
      commit(draft)
      return
    }
    if (event.key === 'Backspace' && draft === '' && value.length > 0) {
      onChange(value.slice(0, -1))
    }
  }

  return (
    <div className="flex items-start gap-2 border-b px-3 py-1.5">
      <Label htmlFor={id} className="mt-1.5 w-10 shrink-0 text-xs text-muted-foreground">
        {label}
      </Label>

      <div className="flex min-w-0 flex-1 flex-wrap items-center gap-1">
        {value.map((address) => (
          <span
            key={address.email}
            className="flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-sm"
          >
            {address.name || address.email}
            <button
              type="button"
              aria-label={`Remove ${address.email}`}
              onClick={() => onChange(value.filter((item) => item.email !== address.email))}
              className="text-muted-foreground hover:text-foreground"
            >
              <X className="size-3" />
            </button>
          </span>
        ))}

        <Input
          id={id}
          value={draft}
          onChange={(event) => {
            setDraft(event.target.value)
            setError(null)
          }}
          onKeyDown={onKeyDown}
          // Committing on blur means an address left in the field is not lost
          // when the user clicks straight to Send.
          onBlur={() => commit(draft)}
          className={cn(
            'h-7 min-w-40 flex-1 border-0 px-1 shadow-none focus-visible:ring-0',
            error && 'text-destructive',
          )}
        />
      </div>

      {error ? (
        <span role="alert" className="mt-1.5 text-xs text-destructive">
          {error}
        </span>
      ) : null}
    </div>
  )
}
