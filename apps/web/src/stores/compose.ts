import { create } from 'zustand'

interface ComposeState {
  /** The draft currently open in the compose window, if any. */
  openDraftId: string | null
  open: (draftId: string) => void
  close: () => void
}

/**
 * Which draft the compose window is showing. UI state only — the draft's
 * contents live on the server and are read through TanStack Query.
 */
export const useComposeStore = create<ComposeState>()((set) => ({
  openDraftId: null,
  open: (draftId) => set({ openDraftId: draftId }),
  close: () => set({ openDraftId: null }),
}))
