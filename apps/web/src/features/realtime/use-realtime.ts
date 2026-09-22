import { useEffect, useRef } from 'react'
import { useQueryClient } from '@tanstack/react-query'

import { draftKeys } from '@/features/compose/api'
import { mailboxKeys } from '@/features/mailbox/api'

/** Events the server pushes. Mirrors internal/realtime/events.go. */
type RealtimeEventType = 'email.received' | 'email.updated' | 'thread.updated' | 'draft.updated'

interface RealtimeEvent {
  type: RealtimeEventType
  mail_account_id?: string
  data?: Record<string, unknown>
  at: string
}

/** Reconnect backoff, in milliseconds, capped so a long outage still recovers. */
const BACKOFF_MS = [1000, 2000, 5000, 10_000, 30_000]

/**
 * Keeps the mailbox live.
 *
 * Events carry only ids, and the response is always to invalidate the relevant
 * queries rather than to patch the cache: one inbound message can change a
 * thread list, an unread count and a sidebar badge at once, and reproducing all
 * of that client-side is how lists drift out of sync with the server.
 */
export function useRealtime(enabled: boolean) {
  const queryClient = useQueryClient()
  // Held in a ref so reconnecting does not re-run the effect.
  const attemptRef = useRef(0)

  useEffect(() => {
    if (!enabled) return

    let socket: WebSocket | null = null
    let reconnectTimer = 0
    let closed = false

    const connect = () => {
      if (closed) return

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      socket = new WebSocket(`${protocol}//${window.location.host}/api/v1/ws`)

      socket.onopen = () => {
        attemptRef.current = 0
      }

      socket.onmessage = (message) => {
        let event: RealtimeEvent
        try {
          event = JSON.parse(message.data as string) as RealtimeEvent
        } catch {
          return
        }

        switch (event.type) {
          case 'email.received':
          case 'email.updated':
          case 'thread.updated':
            void queryClient.invalidateQueries({ queryKey: mailboxKeys.all })
            break
          case 'draft.updated':
            void queryClient.invalidateQueries({ queryKey: draftKeys.all })
            break
        }
      }

      socket.onclose = () => {
        if (closed) return
        // A dropped socket is normal — a laptop sleeping, a proxy timing out.
        // Backing off means a server restart does not get hammered.
        const delay = BACKOFF_MS[Math.min(attemptRef.current, BACKOFF_MS.length - 1)]!
        attemptRef.current += 1
        reconnectTimer = window.setTimeout(connect, delay)
      }

      socket.onerror = () => {
        socket?.close()
      }
    }

    connect()

    return () => {
      closed = true
      window.clearTimeout(reconnectTimer)
      socket?.close()
    }
  }, [enabled, queryClient])
}
