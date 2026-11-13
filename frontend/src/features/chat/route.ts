import { create } from 'zustand'
import type { ThreadType } from '../../api/schemas'

type ChatRoute = {
  /** A thread type another screen wants the chat to open; the chat takes it and clears it. */
  wanted: ThreadType | null
  /** A specific thread another screen shows, e.g. the task on /task/:id. */
  wantedId: string | null
  open: (type: ThreadType) => void
  openId: (id: string) => void
  clear: () => void
}

export const useChatRoute = create<ChatRoute>()((set) => ({
  wanted: null,
  wantedId: null,
  open: (type) => set({ wanted: type, wantedId: null }),
  openId: (id) => set({ wantedId: id, wanted: null }),
  clear: () => set({ wanted: null, wantedId: null }),
}))
