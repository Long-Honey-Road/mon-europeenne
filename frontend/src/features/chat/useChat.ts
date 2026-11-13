import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api'
import type { AnswerMode, Rating } from '../../api/client'
import { openThreadEvents } from '../../api/events'
import type { ThreadEvent } from '../../api/events'
import { blockSchema, escalationStepSchema } from '../../api/schemas'
import type { Block, EscalationStep, ThreadType } from '../../api/schemas'
import { useSession } from '../auth/session'
import { applyRequestStatus } from '../requests/useRequests'
import { useChatRoute } from './route'

/** How long to wait for the event stream before sending, and for the answer after sending. */
export const CONNECT_TIMEOUT_MS = 5_000
export const TURN_TIMEOUT_MS = 120_000
const RECONNECT_MS = 2_000

export type TurnError = { message: string; escalation: EscalationStep[] }

/** The turn in progress: the question, then what streams in until the saved answer replaces it. */
export type LiveTurn = {
  question: string
  status?: string
  text: string
  blocks: Block[]
  error?: TurnError
  /** Set by done: the turn is hidden once this message is in the history. */
  savedId?: string
}

export const threadsKey = ['threads'] as const
export const messagesKey = (threadId: string | null) => ['messages', threadId] as const

const failed = (message: string): TurnError => ({ message, escalation: [] })

function withTimeout(p: Promise<void>, ms: number): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('timeout')), ms)
    p.then(
      () => {
        clearTimeout(timer)
        resolve()
      },
      (e: unknown) => {
        clearTimeout(timer)
        reject(e)
      },
    )
  })
}

export function useChat() {
  const token = useSession((s) => s.token)
  const qc = useQueryClient()
  const [threadId, setThreadId] = useState<string | null>(null)
  const [live, setLive] = useState<LiveTurn | null>(null)
  const [ratings, setRatings] = useState<Record<string, Rating>>({})
  const ready = useRef<Promise<void> | null>(null)
  const turnTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const creating = useRef(false)

  const threads = useQuery({ queryKey: threadsKey, queryFn: api.threads, enabled: token !== null })
  const messages = useQuery({
    queryKey: messagesKey(threadId),
    queryFn: () => api.messages(threadId as string),
    enabled: threadId !== null,
  })

  const createThread = useCallback(
    async (type: ThreadType) => {
      const t = await api.createThread(type)
      await qc.invalidateQueries({ queryKey: threadsKey })
      setLive(null)
      setThreadId(t.id)
      return t
    },
    [qc],
  )

  // Open the newest general thread, or start one.
  useEffect(() => {
    if (threadId || !threads.data || creating.current) return
    const general = threads.data.find((t) => t.type === 'THREAD_TYPE_GENERAL')
    if (general) {
      setThreadId(general.id)
      return
    }
    creating.current = true
    createThread('THREAD_TYPE_GENERAL')
      .catch(() => {})
      .finally(() => {
        creating.current = false
      })
  }, [threadId, threads.data, createThread])

  // Another screen asked for a thread of some type: open the newest one, or start it.
  const wanted = useChatRoute((s) => s.wanted)
  const clearWanted = useChatRoute((s) => s.clear)
  useEffect(() => {
    if (!wanted || !threads.data) return
    clearWanted()
    const found = threads.data.find((t) => t.type === wanted)
    if (found) {
      setLive(null)
      setThreadId(found.id)
      return
    }
    createThread(wanted).catch(() => {})
  }, [wanted, threads.data, createThread, clearWanted])

  const stopTimer = () => {
    if (turnTimer.current) clearTimeout(turnTimer.current)
    turnTimer.current = null
  }

  const onEvent = useCallback(
    (e: ThreadEvent) => {
      const text = typeof e.data.text === 'string' ? e.data.text : ''
      switch (e.type) {
        case 'status':
          setLive((l) => l && { ...l, status: text })
          break
        case 'delta':
          setLive((l) => l && { ...l, status: undefined, text: l.text + text })
          break
        case 'reset':
          setLive((l) => l && { ...l, text: '' })
          break
        case 'block': {
          const block = blockSchema.safeParse(e.data.block)
          if (block.success) setLive((l) => l && { ...l, blocks: [...l.blocks, block.data] })
          break
        }
        case 'done': {
          stopTimer()
          // The saved answer replaces the stream: the server may have removed invented links.
          const savedId = typeof e.data.message_id === 'string' ? e.data.message_id : ''
          setLive((l) => l && { ...l, status: undefined, savedId })
          void qc.invalidateQueries({ queryKey: messagesKey(threadId) })
          break
        }
        case 'request_status':
          applyRequestStatus(qc, e.data)
          break
        case 'error': {
          stopTimer()
          const escalation = escalationStepSchema.array().safeParse(e.data.escalation)
          const message = typeof e.data.message === 'string' ? e.data.message : 'Не удалось ответить'
          setLive((l) => l && { ...l, status: undefined, error: { message, escalation: escalation.success ? escalation.data : [] } })
          void qc.invalidateQueries({ queryKey: messagesKey(threadId) })
          break
        }
      }
    },
    [qc, threadId],
  )

  // One stream per open thread, reconnected while the thread stays open.
  useEffect(() => {
    if (!threadId || !token) return
    const ctrl = new AbortController()
    const connect = () => {
      const stream = openThreadEvents({
        baseUrl: import.meta.env.VITE_API_URL ?? '',
        token,
        threadId,
        signal: ctrl.signal,
        onEvent,
      })
      ready.current = stream.ready
      stream.closed
        .catch(() => {})
        .finally(() => {
          if (!ctrl.signal.aborted) setTimeout(() => !ctrl.signal.aborted && connect(), RECONNECT_MS)
        })
    }
    connect()
    return () => {
      ctrl.abort()
      ready.current = null
    }
  }, [threadId, token, onEvent])

  useEffect(() => stopTimer, [])

  // Drop the streamed turn in the same render the saved answer appears.
  const history = messages.data
  const saved = live?.savedId !== undefined && (live.savedId === '' || (history ?? []).some((m) => m.id === live.savedId))
  useEffect(() => {
    if (saved) setLive(null)
  }, [saved])
  const shownLive = saved ? null : live

  const busy = live !== null && !live.error

  const send = useCallback(
    async (question: string, mode: AnswerMode) => {
      const text = question.trim()
      if (!text || !threadId || busy) return
      setLive({ question: text, text: '', blocks: [] })
      try {
        await withTimeout(ready.current ?? Promise.reject(new Error('no stream')), CONNECT_TIMEOUT_MS)
      } catch {
        setLive((l) => l && { ...l, error: failed('Нет связи с сервером: ответ не придёт. Попробуй ещё раз.') })
        return
      }
      try {
        await api.sendMessage(threadId, text, mode)
      } catch {
        setLive((l) => l && { ...l, error: failed('Сообщение не отправилось. Попробуй ещё раз.') })
        return
      }
      stopTimer()
      turnTimer.current = setTimeout(() => {
        setLive((l) => l && !l.error ? { ...l, status: undefined, error: failed('Ответ не пришёл вовремя. Попробуй ещё раз.') } : l)
        void qc.invalidateQueries({ queryKey: messagesKey(threadId) })
      }, TURN_TIMEOUT_MS)
    },
    [threadId, busy, qc],
  )

  const rate = useCallback(async (messageId: string, value: Rating) => {
    setRatings((r) => ({ ...r, [messageId]: value }))
    try {
      await api.rateMessage(messageId, value)
    } catch {
      setRatings((r) => {
        const next = { ...r }
        delete next[messageId]
        return next
      })
    }
  }, [])

  const selectThread = useCallback((id: string) => {
    setLive(null)
    setThreadId(id)
  }, [])

  return {
    threads: threads.data ?? [],
    threadId,
    selectThread,
    createThread,
    messages: messages.data ?? [],
    loading: threads.isLoading || messages.isLoading,
    loadError: threads.isError || messages.isError,
    live: shownLive,
    busy,
    send,
    ratings,
    rate,
  }
}
