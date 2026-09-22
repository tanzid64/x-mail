import { create } from 'zustand'
import { persist } from 'zustand/middleware'

/**
 * "all" is the unified inbox across every account; a uuid selects one account.
 */
export type AccountSelection = 'all' | string

interface UIState {
  /** Which mail account the mailbox is showing. */
  activeAccount: AccountSelection
  setActiveAccount: (selection: AccountSelection) => void
}

/**
 * UI-only state. Server data lives in TanStack Query — nothing that the API
 * owns belongs here.
 *
 * The selection is persisted so reopening the app returns to the account the
 * user was last reading, the way Gmail remembers the active profile.
 */
export const useUIStore = create<UIState>()(
  persist(
    (set) => ({
      activeAccount: 'all',
      setActiveAccount: (selection) => set({ activeAccount: selection }),
    }),
    { name: 'x-mail-ui' },
  ),
)
