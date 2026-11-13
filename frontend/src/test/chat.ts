import { HttpResponse, http } from 'msw'
import type { ChatMessage, Thread, ThreadType } from '../api/schemas'

// A fake chat backend: threads, history and an SSE stream that plays a scripted turn after each message.
const API = 'http://api.test'

export type ScriptEvent = { event: string; data: Record<string, unknown> }

type State = {
  threads: Thread[]
  messages: Record<string, ChatMessage[]>
  /** Events sent after each message; a done event also saves the answer into the history. */
  script: ScriptEvent[]
  sent: { threadId: string; content: string; mode: string }[]
  ratings: { messageId: string; value: string }[]
  failEvents: boolean
  failSend: boolean
  seq: number
}

const fresh = (): State => ({
  threads: [],
  messages: {},
  script: [],
  sent: [],
  ratings: [],
  failEvents: false,
  failSend: false,
  seq: 0,
})

// A turn scheduled by one test must not play into the next one, whose thread ids start over.
let generation = 0

export const chat: State & { reset: () => void } = Object.assign(fresh(), {
  reset() {
    generation++
    Object.assign(chat, fresh())
    streams.clear()
  },
})

const streams = new Map<string, ReadableStreamDefaultController<Uint8Array>>()
const encoder = new TextEncoder()

const nextId = () => `00000000-0000-0000-0000-${String(++chat.seq).padStart(12, '0')}`

export function addThread(type: ThreadType = 'THREAD_TYPE_GENERAL', createdAt = '2026-11-13T09:00:00Z'): Thread {
  const t: Thread = { id: nextId(), type, title: '', created_at: createdAt }
  chat.threads.unshift(t)
  chat.messages[t.id] = []
  return t
}

export function addMessage(threadId: string, role: 'user' | 'assistant', text: string, blocks: ChatMessage['blocks'] = []) {
  const m: ChatMessage = {
    id: nextId(),
    thread_id: threadId,
    role: role === 'user' ? 'MESSAGE_ROLE_USER' : 'MESSAGE_ROLE_ASSISTANT',
    text,
    blocks,
    created_at: '2026-11-13T09:01:00Z',
  }
  chat.messages[threadId] = [...(chat.messages[threadId] ?? []), m]
  return m
}

/** A typical answer: a status, two deltas, a person block, then done with the saved text. */
export const answer = (text: string): ScriptEvent[] => [
  { event: 'status', data: { job_id: 'j', text: 'ищу, к кому обратиться' } },
  { event: 'delta', data: { job_id: 'j', text: text.slice(0, 5) } },
  { event: 'delta', data: { job_id: 'j', text: text.slice(5) } },
  {
    event: 'block',
    data: { job_id: 'j', block: { type: 'person', person_id: 'p-igor', name: 'Игорь Волков', reason: 'деплой', contact: '@igor' } },
  },
  { event: 'done', data: { job_id: 'j', text } },
]

/** Sends an event to the open stream of a thread, outside any turn (e.g. request_status). */
export function emit(threadId: string, e: ScriptEvent) {
  push(threadId, e)
}

function push(threadId: string, e: ScriptEvent) {
  streams.get(threadId)?.enqueue(encoder.encode(`event: ${e.event}\ndata: ${JSON.stringify(e.data)}\n\n`))
}

function play(threadId: string) {
  const blocks: ChatMessage['blocks'] = []
  for (const e of chat.script) {
    if (e.event === 'block') blocks.push(e.data.block as ChatMessage['blocks'][number])
    if (e.event === 'done') {
      const saved = addMessage(threadId, 'assistant', String(e.data.text), blocks)
      e.data.message_id = saved.id
    }
    push(threadId, e)
  }
}

export const chatHandlers = [
  http.get(`${API}/v1/private/threads`, () => HttpResponse.json({ threads: chat.threads })),
  http.post(`${API}/v1/private/threads`, async ({ request }) => {
    const body = (await request.json()) as { type: ThreadType }
    return HttpResponse.json({ thread: addThread(body.type, '2026-11-13T10:00:00Z') })
  }),
  http.get(`${API}/v1/private/threads/:id/messages`, ({ params }) =>
    HttpResponse.json({ messages: chat.messages[String(params.id)] ?? [] }),
  ),
  http.get(`${API}/v1/private/threads/:id/events`, ({ params, request }) => {
    if (chat.failEvents) return HttpResponse.json({ code: 14, message: 'down', details: [] }, { status: 503 })
    const id = String(params.id)
    const stream = new ReadableStream<Uint8Array>({
      start(c) {
        streams.set(id, c)
        c.enqueue(encoder.encode(': connected\n\n'))
        request.signal.addEventListener('abort', () => {
          if (streams.get(id) === c) streams.delete(id)
          try {
            c.close()
          } catch {
            // already closed
          }
        })
      },
    })
    return new HttpResponse(stream, { headers: { 'Content-Type': 'text/event-stream' } })
  }),
  http.post(`${API}/v1/private/threads/:id/messages`, async ({ params, request }) => {
    if (chat.failSend) return HttpResponse.json({ code: 14, message: 'down', details: [] }, { status: 503 })
    const id = String(params.id)
    const body = (await request.json()) as { content: string; mode: string }
    chat.sent.push({ threadId: id, content: body.content, mode: body.mode })
    const msg = addMessage(id, 'user', body.content)
    const scheduled = generation
    setTimeout(() => scheduled === generation && play(id), 10)
    return HttpResponse.json({ message_id: msg.id, job_id: 'j' }, { status: 202 })
  }),
  http.post(`${API}/v1/private/messages/:id/feedback`, async ({ params, request }) => {
    const body = (await request.json()) as { value: string }
    chat.ratings.push({ messageId: String(params.id), value: body.value })
    return HttpResponse.json({})
  }),
]
