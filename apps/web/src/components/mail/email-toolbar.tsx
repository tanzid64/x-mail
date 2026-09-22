import {
  Archive,
  Inbox,
  MailOpen,
  Mail as MailIcon,
  RefreshCw,
  ShieldAlert,
  ShieldCheck,
  Trash2,
} from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Separator } from '@/components/ui/separator'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import type { ThreadAction } from '@/features/mailbox/types'

interface EmailToolbarProps {
  folder: string
  selectedCount: number
  totalCount: number
  allSelected: boolean
  onSelectAllChange: (selected: boolean) => void
  onAction: (action: ThreadAction) => void
  onRefresh: () => void
  busy: boolean
}

/**
 * Actions above the list. Which actions make sense depends on where you are:
 * archiving from the archive, or reporting spam from the spam folder, would
 * do nothing.
 */
export function EmailToolbar({
  folder,
  selectedCount,
  totalCount,
  allSelected,
  onSelectAllChange,
  onAction,
  onRefresh,
  busy,
}: EmailToolbarProps) {
  const hasSelection = selectedCount > 0
  const inTrash = folder === 'trash'
  const inSpam = folder === 'spam'
  const inArchive = folder === 'archive'

  const actions: { action: ThreadAction; label: string; icon: typeof Archive; show: boolean }[] = [
    { action: 'archive', label: 'Archive', icon: Archive, show: !inArchive && !inTrash && !inSpam },
    { action: 'inbox', label: 'Move to inbox', icon: Inbox, show: inArchive || inTrash || inSpam },
    { action: 'spam', label: 'Report spam', icon: ShieldAlert, show: !inSpam && !inTrash },
    { action: 'not_spam', label: 'Not spam', icon: ShieldCheck, show: inSpam },
    { action: 'trash', label: 'Move to trash', icon: Trash2, show: !inTrash },
    { action: 'delete', label: 'Delete forever', icon: Trash2, show: inTrash },
    { action: 'read', label: 'Mark as read', icon: MailOpen, show: true },
    { action: 'unread', label: 'Mark as unread', icon: MailIcon, show: true },
  ]

  return (
    <div className="flex h-12 shrink-0 items-center gap-1 border-b px-4">
      <Checkbox
        checked={allSelected && totalCount > 0}
        onCheckedChange={(value) => onSelectAllChange(value === true)}
        disabled={totalCount === 0}
        aria-label="Select all conversations"
      />

      <Separator orientation="vertical" className="mx-2 h-5" />

      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="icon" onClick={onRefresh} aria-label="Refresh" disabled={busy}>
            <RefreshCw className="size-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Refresh</TooltipContent>
      </Tooltip>

      {hasSelection ? (
        <>
          <Separator orientation="vertical" className="mx-2 h-5" />
          {actions
            .filter((entry) => entry.show)
            .map((entry) => (
              <Tooltip key={entry.action}>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={entry.label}
                    disabled={busy}
                    onClick={() => onAction(entry.action)}
                  >
                    <entry.icon className="size-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{entry.label}</TooltipContent>
              </Tooltip>
            ))}

          <span className="ml-2 text-sm text-muted-foreground">
            {selectedCount} selected
          </span>
        </>
      ) : null}
    </div>
  )
}
