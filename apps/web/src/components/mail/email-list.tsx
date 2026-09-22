import { useMemo, useState } from 'react'
import { Inbox, Loader2 } from 'lucide-react'

import { EmailRow } from '@/components/mail/email-row'
import { EmailToolbar } from '@/components/mail/email-toolbar'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useThreadAction, useThreads } from '@/features/mailbox/api'
import type { ThreadAction } from '@/features/mailbox/types'
import { useUIStore } from '@/stores/ui'

interface EmailListProps {
  folder: string
  labelId?: string
  emptyMessage: string
}

export function EmailList({ folder, labelId, emptyMessage }: EmailListProps) {
  const account = useUIStore((state) => state.activeAccount)
  const { data, isLoading, error, fetchNextPage, hasNextPage, isFetchingNextPage, refetch, isFetching } =
    useThreads({ folder, account, labelId })
  const act = useThreadAction()

  const [selected, setSelected] = useState<Set<string>>(new Set())

  // A selection cannot outlive the rows it referred to — switching folder or
  // account replaces the list entirely. Resetting during render rather than in
  // an effect avoids rendering one frame with a selection that points at rows
  // no longer on screen.
  const listKey = `${folder}|${account}|${labelId ?? ''}`
  const [renderedKey, setRenderedKey] = useState(listKey)
  if (renderedKey !== listKey) {
    setRenderedKey(listKey)
    setSelected(new Set())
  }

  const threads = useMemo(() => data?.pages.flatMap((page) => page.threads) ?? [], [data])

  const toggle = (id: string, isSelected: boolean) => {
    setSelected((current) => {
      const next = new Set(current)
      if (isSelected) next.add(id)
      else next.delete(id)
      return next
    })
  }

  const runAction = (action: ThreadAction, ids: string[]) => {
    if (ids.length === 0) return
    act.mutate({ action, threadIds: ids }, { onSuccess: () => setSelected(new Set()) })
  }

  if (isLoading) {
    return (
      <div className="flex flex-col gap-px p-4">
        {Array.from({ length: 6 }).map((_, index) => (
          <Skeleton key={index} className="h-12 w-full" />
        ))}
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex flex-col items-center gap-3 p-10 text-center">
        <p className="text-sm text-destructive">{error.message}</p>
        <Button variant="outline" onClick={() => void refetch()}>
          Try again
        </Button>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col">
      <EmailToolbar
        folder={folder}
        selectedCount={selected.size}
        totalCount={threads.length}
        allSelected={threads.length > 0 && selected.size === threads.length}
        onSelectAllChange={(all) =>
          setSelected(all ? new Set(threads.map((thread) => thread.id)) : new Set())
        }
        onAction={(action) => runAction(action, [...selected])}
        onRefresh={() => void refetch()}
        busy={act.isPending || isFetching}
      />

      {threads.length === 0 ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 p-10 text-center">
          <Inbox className="size-6 text-muted-foreground" />
          <p className="font-medium">Nothing here</p>
          <p className="max-w-sm text-sm text-muted-foreground">{emptyMessage}</p>
        </div>
      ) : (
        <div className="flex-1 overflow-auto">
          {threads.map((thread) => (
            <EmailRow
              key={thread.id}
              thread={thread}
              selected={selected.has(thread.id)}
              onSelectedChange={(value) => toggle(thread.id, value)}
              onToggleStar={() => runAction(thread.is_starred ? 'unstar' : 'star', [thread.id])}
              showAccount={account === 'all'}
            />
          ))}

          {hasNextPage ? (
            <div className="flex justify-center p-4">
              <Button variant="outline" onClick={() => void fetchNextPage()} disabled={isFetchingNextPage}>
                {isFetchingNextPage ? <Loader2 className="size-4 animate-spin" /> : null}
                Load more
              </Button>
            </div>
          ) : null}
        </div>
      )}
    </div>
  )
}
