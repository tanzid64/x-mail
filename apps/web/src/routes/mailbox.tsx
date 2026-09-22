import { useParams } from '@tanstack/react-router'

import { folderBySlug } from '@/lib/folders'

/**
 * Placeholder mailbox view.
 *
 * Routing, the shell and the folder list are real; the thread list itself
 * arrives with the mailbox phase, once threads and messages exist server-side.
 */
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
      <div className="flex items-center gap-2 border-b px-6 py-4">
        <folder.icon className="size-4 text-muted-foreground" />
        <h1 className="text-lg font-semibold">{folder.label}</h1>
      </div>

      <div className="flex flex-1 items-center justify-center p-10">
        <div className="max-w-sm text-center">
          <p className="text-sm font-medium">No messages yet</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Connect a mail account to start sending and receiving from this mailbox.
          </p>
        </div>
      </div>
    </div>
  )
}
