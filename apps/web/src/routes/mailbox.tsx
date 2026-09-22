import { useParams } from '@tanstack/react-router'

import { EmailList } from '@/components/mail/email-list'
import { folderBySlug } from '@/lib/folders'

/** Empty-state copy per folder, so the message fits where you are. */
const EMPTY_MESSAGES: Record<string, string> = {
  inbox: 'New mail will appear here once an account is connected and receiving.',
  starred: 'Star a conversation to keep it here.',
  snoozed: 'Snoozed conversations come back here at the time you choose.',
  sent: 'Messages you send will appear here.',
  drafts: 'Drafts you start will be saved here automatically.',
  archive: 'Archived conversations are kept here, out of the inbox.',
  spam: 'Mail marked as spam lands here.',
  trash: 'Deleted conversations stay here until you remove them for good.',
}

export function MailboxPage() {
  const { folder: slug } = useParams({ from: '/_app/$folder' })
  const folder = folderBySlug(slug)

  if (!folder) {
    return (
      <div className="p-6">
        <h1 className="text-lg font-semibold">Folder not found</h1>
        <p className="mt-1 text-sm text-muted-foreground">There is no “{slug}” folder.</p>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2 border-b px-4 py-3">
        <folder.icon className="size-4 text-muted-foreground" />
        <h1 className="font-semibold">{folder.label}</h1>
      </div>

      <div className="min-h-0 flex-1">
        <EmailList
          folder={folder.slug}
          emptyMessage={EMPTY_MESSAGES[folder.slug] ?? 'Nothing to show here yet.'}
        />
      </div>
    </div>
  )
}
