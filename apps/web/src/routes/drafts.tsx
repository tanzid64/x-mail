import { FileText, Loader2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useDrafts } from '@/features/compose/api'
import { useStartCompose } from '@/features/compose/use-start-compose'
import { formatMailDate } from '@/lib/format'
import { useComposeStore } from '@/stores/compose'
import { useUIStore } from '@/stores/ui'

/**
 * Drafts are listed from the drafts table rather than from threads: an unsent
 * draft is not a message yet, and keeping it out of the mailbox tables is what
 * stops autosave churning the thread aggregates.
 */
export function DraftsPage() {
  const account = useUIStore((state) => state.activeAccount)
  const { data: drafts, isLoading } = useDrafts(account)
  const openDraft = useComposeStore((state) => state.open)
  const { start, canCompose, isPending } = useStartCompose()

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2 border-b px-4 py-3">
        <FileText className="size-4 text-muted-foreground" />
        <h1 className="font-semibold">Drafts</h1>
        <Button
          size="sm"
          variant="outline"
          className="ml-auto"
          disabled={!canCompose || isPending}
          onClick={() => void start()}
        >
          {isPending ? <Loader2 className="size-3.5 animate-spin" /> : null}
          New message
        </Button>
      </div>

      {isLoading ? (
        <div className="flex flex-col gap-px p-4">
          {Array.from({ length: 4 }).map((_, index) => (
            <Skeleton key={index} className="h-12 w-full" />
          ))}
        </div>
      ) : drafts && drafts.length > 0 ? (
        <div className="flex-1 overflow-auto">
          {drafts.map((draft) => (
            <button
              key={draft.id}
              type="button"
              data-testid="draft-row"
              onClick={() => openDraft(draft.id)}
              className="flex w-full items-center gap-3 border-b px-4 py-2 text-left transition hover:bg-muted/60"
            >
              <span className="w-40 shrink-0 truncate text-sm text-muted-foreground">
                {draft.to.length > 0 ? draft.to.map((address) => address.email).join(', ') : 'No recipient'}
              </span>
              <span className="min-w-0 flex-1 truncate text-sm">
                {draft.subject || '(no subject)'}
                {draft.text_body ? (
                  <span className="ml-2 text-muted-foreground">— {draft.text_body.slice(0, 120)}</span>
                ) : null}
              </span>
              <span className="w-20 shrink-0 text-right text-xs text-muted-foreground">
                {formatMailDate(draft.updated_at)}
              </span>
            </button>
          ))}
        </div>
      ) : (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 p-10 text-center">
          <FileText className="size-6 text-muted-foreground" />
          <p className="font-medium">No drafts</p>
          <p className="max-w-sm text-sm text-muted-foreground">
            Messages you start but do not send are saved here automatically.
          </p>
        </div>
      )}
    </div>
  )
}
