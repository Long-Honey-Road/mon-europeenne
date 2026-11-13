import { create } from 'zustand'
import type { ThreadType } from '../../api/schemas'

type ChatRoute = {
  /** A thread type another screen wants the chat to open; the chat takes it and clears it. */
  wanted: ThreadType | null
  open: (type: ThreadType) => void
  clear: () => void
}

export const useChatRoute = create<ChatRoute>()((set) => ({
  wanted: null,
  open: (type) => set({ wanted: type }),
  clear: () => set({ wanted: null }),
}))
