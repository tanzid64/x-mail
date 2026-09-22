import { Archive, File, Inbox, Send, Star, Trash2, Clock, ShieldAlert } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

/**
 * The system folders in the sidebar. These map onto the system labels the API
 * creates for every mail account (INBOX, SENT, DRAFT, …), so the route segment
 * is the single name used by the UI, the router and the API query.
 */
export interface Folder {
  slug: string
  label: string
  icon: LucideIcon
}

export const FOLDERS: Folder[] = [
  { slug: 'inbox', label: 'Inbox', icon: Inbox },
  { slug: 'starred', label: 'Starred', icon: Star },
  { slug: 'snoozed', label: 'Snoozed', icon: Clock },
  { slug: 'sent', label: 'Sent', icon: Send },
  { slug: 'drafts', label: 'Drafts', icon: File },
  { slug: 'archive', label: 'Archive', icon: Archive },
  { slug: 'spam', label: 'Spam', icon: ShieldAlert },
  { slug: 'trash', label: 'Trash', icon: Trash2 },
]

export function folderBySlug(slug: string): Folder | undefined {
  return FOLDERS.find((folder) => folder.slug === slug)
}
