import { Loader2, Paperclip, X } from 'lucide-react'

import { Button } from '@/components/ui/button'
import type { Attachment } from '@/features/compose/types'

interface AttachmentListProps {
  attachments: Attachment[]
  onRemove: (attachmentId: string) => void
  uploading: boolean
  removing: boolean
}

/** Human-readable size, since a byte count means nothing at a glance. */
export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB']
  let value = bytes / 1024
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  return `${value.toFixed(value >= 10 ? 0 : 1)} ${units[unit]}`
}

export function AttachmentList({ attachments, onRemove, uploading, removing }: AttachmentListProps) {
  if (attachments.length === 0 && !uploading) return null

  return (
    <div className="flex flex-wrap gap-2 border-t px-3 py-2">
      {attachments.map((attachment) => (
        <span
          key={attachment.id}
          className="flex items-center gap-2 rounded-md border px-2 py-1 text-sm"
          data-testid="attachment-chip"
        >
          <Paperclip className="size-3.5 shrink-0 text-muted-foreground" />
          <span className="max-w-48 truncate">{attachment.filename}</span>
          <span className="text-xs text-muted-foreground">{formatBytes(attachment.size_bytes)}</span>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-5"
            aria-label={`Remove ${attachment.filename}`}
            disabled={removing}
            onClick={() => onRemove(attachment.id)}
          >
            <X className="size-3" />
          </Button>
        </span>
      ))}

      {uploading ? (
        <span className="flex items-center gap-2 rounded-md border border-dashed px-2 py-1 text-sm text-muted-foreground">
          <Loader2 className="size-3.5 animate-spin" />
          Uploading…
        </span>
      ) : null}
    </div>
  )
}
