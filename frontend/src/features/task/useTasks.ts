import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { api } from '../../api'
import type { TaskPin, Thread } from '../../api/schemas'
import { useSession } from '../auth/session'
import { threadsKey } from '../chat/useChat'

/** All threads, shared with the chat through the same query key. */
export function useThreads() {
  const signedIn = useSession((s) => s.token !== null)
  return useQuery({ queryKey: threadsKey, queryFn: api.threads, enabled: signedIn })
}

export const taskOf = (t: Thread): TaskPin | undefined => (t.type === 'THREAD_TYPE_TASK' ? t.pin.task : undefined)

/** Starts a task thread and opens its screen. */
export function useStartTask() {
  const qc = useQueryClient()
  const navigate = useNavigate()
  return useMutation({
    mutationFn: (task: TaskPin) =>
      api.createThread('THREAD_TYPE_TASK', { title: task.title ? `${task.key} · ${task.title}` : task.key, task }),
    onSuccess: async (t) => {
      await qc.invalidateQueries({ queryKey: threadsKey })
      void navigate({ to: '/task/$threadId', params: { threadId: t.id } })
    },
  })
}
