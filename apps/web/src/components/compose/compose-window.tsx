import { useEffect, useRef, useState } from 'react'
import { Loader2, Paperclip, Send, Trash2, X } from 'lucide-react'

import { AttachmentList } from '@/components/compose/attachment-list'
import { RecipientInput } from '@/components/compose/recipient-input'
import { RichTextEditor } from '@/components/compose/rich-text-editor'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useAddresses } from '@/features/accounts/api'
import {
  useDeleteAttachment,
  useDeleteDraft,
  useDraft,
  useSaveDraft,
  useSendDraft,
  useUploadAttachment,
} from '@/features/compose/api'
import type { ComposeFields } from '@/features/compose/types'
import { useDebouncedEffect } from '@/hooks/use-debounced-effect'
import { ApiError } from '@/lib/api'
import { useComposeStore } from '@/stores/compose'

/** How long typing must pause before the draft is saved. */
const AUTOSAVE_DELAY_MS = 2000

export function ComposeWindow() {
  const draftId = useComposeStore((state) => state.openDraftId)
  const close = useComposeStore((state) => state.close)

  if (!draftId) return null
  // Keyed on the draft so opening a different one starts with clean state
  // rather than the previous message's text.
  return <ComposeWindowInner key={draftId} draftId={draftId} onClose={close} />
}

interface InnerProps {
  draftId: string
  onClose: () => void
}

function ComposeWindowInner({ draftId, onClose }: InnerProps) {
  const { data: draft, isLoading } = useDraft(draftId)
  const { data: addresses } = useAddresses()

  const save = useSaveDraft()
  const send = useSendDraft()
  const remove = useDeleteDraft()
  const upload = useUploadAttachment()
  const deleteAttachment = useDeleteAttachment()

  const fileInput = useRef<HTMLInputElement>(null)
  const [fields, setFields] = useState<ComposeFields | null>(null)
  const [showCc, setShowCc] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [savedAt, setSavedAt] = useState<Date | null>(null)

  // Seed the form once the draft arrives. Later autosaves must not overwrite
  // what the user is currently typing.
  useEffect(() => {
    if (!draft || fields !== null) return
    setFields({
      from_address_id: draft.from_address_id,
      to: draft.to,
      cc: draft.cc,
      bcc: draft.bcc,
      subject: draft.subject,
      html_body: draft.html_body,
      text_body: draft.text_body,
    })
    setShowCc(draft.cc.length > 0 || draft.bcc.length > 0)
  }, [draft, fields])

  // Autosave. A message already on its way out must not be edited.
  useDebouncedEffect(
    () => {
      if (!fields || !draft || draft.status !== 'draft') return
      save.mutate(
        { id: draftId, ...fields },
        { onSuccess: () => setSavedAt(new Date()) },
      )
    },
    [fields],
    AUTOSAVE_DELAY_MS,
  )

  if (isLoading || !fields || !draft) {
    return (
      <div className="fixed bottom-0 right-6 z-40 flex h-96 w-[34rem] items-center justify-center rounded-t-lg border bg-card shadow-lg">
        <Loader2 className="size-5 animate-spin text-muted-foreground" />
      </div>
    )
  }

  const update = (patch: Partial<ComposeFields>) => {
    setFields((current) => (current ? { ...current, ...patch } : current))
  }

  const usable = (addresses ?? []).filter(
    (address) => address.mail_account_id === draft.mail_account_id && address.is_active,
  )

  const onSend = async () => {
    setError(null)
    try {
      // Flush whatever has not been autosaved yet, so nothing typed in the last
      // couple of seconds is lost.
      await save.mutateAsync({ id: draftId, ...fields })
      await send.mutateAsync(draftId)
      onClose()
    } catch (err) {
      setError(
        err instanceof ApiError
          ? (err.fields.to ?? err.fields.from_address_id ?? err.message)
          : 'Could not send this message.',
      )
    }
  }

  const onDiscard = async () => {
    await remove.mutateAsync(draftId)
    onClose()
  }

  const onPickFiles = (files: FileList | null) => {
    if (!files) return
    for (const file of Array.from(files)) {
      upload.mutate(
        { draftId, file },
        {
          onError: (err) =>
            setError(err instanceof ApiError ? err.message : `Could not attach ${file.name}.`),
        },
      )
    }
    if (fileInput.current) fileInput.current.value = ''
  }

  return (
    <div
      role="dialog"
      aria-label="New message"
      className="fixed bottom-0 right-6 z-40 flex h-[36rem] w-[34rem] max-w-[calc(100vw-2rem)] flex-col rounded-t-lg border bg-card shadow-xl"
    >
      <header className="flex items-center justify-between rounded-t-lg bg-muted px-3 py-2">
        <span className="text-sm font-medium">{fields.subject || 'New message'}</span>
        <Button variant="ghost" size="icon" className="size-7" aria-label="Close" onClick={onClose}>
          <X className="size-4" />
        </Button>
      </header>

      <div className="flex items-center gap-2 border-b px-3 py-1.5">
        <Label htmlFor="compose-from" className="w-10 shrink-0 text-xs text-muted-foreground">
          From
        </Label>
        <Select
          value={fields.from_address_id ?? undefined}
          onValueChange={(value) => update({ from_address_id: value })}
        >
          <SelectTrigger id="compose-from" className="h-7 border-0 shadow-none focus:ring-0">
            <SelectValue placeholder="Choose an address" />
          </SelectTrigger>
          <SelectContent>
            {usable.map((address) => (
              <SelectItem key={address.id} value={address.id}>
                {address.display_name ? `${address.display_name} <${address.email}>` : address.email}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <RecipientInput id="compose-to" label="To" value={fields.to} onChange={(to) => update({ to })} />

      {showCc ? (
        <>
          <RecipientInput id="compose-cc" label="Cc" value={fields.cc} onChange={(cc) => update({ cc })} />
          <RecipientInput id="compose-bcc" label="Bcc" value={fields.bcc} onChange={(bcc) => update({ bcc })} />
        </>
      ) : (
        <button
          type="button"
          onClick={() => setShowCc(true)}
          className="self-start px-3 py-1 text-xs text-muted-foreground underline-offset-4 hover:underline"
        >
          Add Cc / Bcc
        </button>
      )}

      <div className="flex items-center gap-2 border-b px-3 py-1.5">
        <Label htmlFor="compose-subject" className="w-10 shrink-0 text-xs text-muted-foreground">
          Subject
        </Label>
        <Input
          id="compose-subject"
          value={fields.subject}
          onChange={(event) => update({ subject: event.target.value })}
          className="h-7 border-0 px-1 shadow-none focus-visible:ring-0"
        />
      </div>

      <RichTextEditor
        html={fields.html_body}
        onChange={({ html, text }) => update({ html_body: html, text_body: text })}
      />

      <AttachmentList
        attachments={draft.attachments}
        onRemove={(attachmentId) => deleteAttachment.mutate({ attachmentId, draftId })}
        uploading={upload.isPending}
        removing={deleteAttachment.isPending}
      />

      {error ? (
        <p role="alert" className="border-t px-3 py-2 text-sm text-destructive">
          {error}
        </p>
      ) : null}

      <footer className="flex items-center gap-2 border-t px-3 py-2">
        <Button onClick={() => void onSend()} disabled={send.isPending} className="gap-2">
          {send.isPending ? <Loader2 className="size-4 animate-spin" /> : <Send className="size-4" />}
          Send
        </Button>

        <input
          ref={fileInput}
          type="file"
          multiple
          className="hidden"
          aria-label="Attach files"
          onChange={(event) => onPickFiles(event.target.files)}
        />
        <Button
          variant="ghost"
          size="icon"
          aria-label="Attach a file"
          onClick={() => fileInput.current?.click()}
        >
          <Paperclip className="size-4" />
        </Button>

        <span className="ml-auto text-xs text-muted-foreground">
          {save.isPending ? 'Saving…' : savedAt ? 'Draft saved' : null}
        </span>

        <Button
          variant="ghost"
          size="icon"
          aria-label="Discard draft"
          disabled={remove.isPending}
          onClick={() => void onDiscard()}
        >
          <Trash2 className="size-4" />
        </Button>
      </footer>
    </div>
  )
}
