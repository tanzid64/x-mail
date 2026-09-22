/**
 * Mailbox-style timestamp: a time for today, a day and month for this year,
 * and a full date beyond that — the same progression Gmail uses, so a column of
 * them stays scannable.
 */
export function formatMailDate(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''

  const now = new Date()
  const sameDay =
    date.getDate() === now.getDate() &&
    date.getMonth() === now.getMonth() &&
    date.getFullYear() === now.getFullYear()

  if (sameDay) {
    return date.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
  }
  if (date.getFullYear() === now.getFullYear()) {
    return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
  }
  return date.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

/** Full timestamp for a message header, where precision matters. */
export function formatFullDate(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString(undefined, {
    weekday: 'short',
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  })
}

/** "John Smith", or the local part of the address when there is no name. */
export function displayName(name: string, email: string): string {
  const trimmed = name.trim()
  if (trimmed) return trimmed
  const [local] = email.split('@')
  return local ?? email
}

/** Up to two initials for an avatar fallback. */
export function initialsFor(name: string, email: string): string {
  const source = displayName(name, email)
  const parts = source.split(/[\s@._-]+/).filter(Boolean)
  return parts.slice(0, 2).map((part) => part[0]!.toUpperCase()).join('') || '?'
}

/**
 * Condenses a thread's participants the way a mail client does: a couple of
 * names, then a count of the rest.
 */
export function summariseParticipants(participants: { name: string; email: string }[]): string {
  if (participants.length === 0) return '(no sender)'

  const names = participants.map((p) => displayName(p.name, p.email))
  if (names.length <= 2) return names.join(', ')
  return `${names[0]}, ${names[1]} +${names.length - 2}`
}
