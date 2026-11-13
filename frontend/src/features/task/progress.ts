import { create } from 'zustand'
import { createJSONStorage, persist } from 'zustand/middleware'

type TaskProgress = {
  /** Passed step indexes by task thread id. */
  done: Record<string, number[]>
  toggle: (threadId: string, step: number) => void
  reset: () => void
}

const storage = createJSONStorage(() => ({
  getItem: (k: string) => {
    try {
      return localStorage.getItem(k)
    } catch {
      return null
    }
  },
  setItem: (k: string, v: string) => {
    try {
      localStorage.setItem(k, v)
    } catch {
      /* memory only */
    }
  },
  removeItem: (k: string) => {
    try {
      localStorage.removeItem(k)
    } catch {
      /* memory only */
    }
  },
}))

// The intern's own ticks on the steps: a convenience in this browser, not data buddy relies on.
export const useTaskProgress = create<TaskProgress>()(
  persist(
    (set) => ({
      done: {},
      toggle: (threadId, step) =>
        set((s) => {
          const current = s.done[threadId] ?? []
          const next = current.includes(step) ? current.filter((x) => x !== step) : [...current, step]
          return { done: { ...s.done, [threadId]: next } }
        }),
      reset: () => set({ done: {} }),
    }),
    { name: 'buddy-task-progress', storage },
  ),
)
