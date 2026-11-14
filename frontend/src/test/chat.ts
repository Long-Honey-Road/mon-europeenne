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
  /** Bodies of POST /threads, to check what a screen asked to create. */
  created: { type: ThreadType; title?: string; pin?: Thread['pin'] }[]
  failEvents: boolean
  failSend: boolean
  /** A turn is still running in the thread: sending answers 409. */
  busy: boolean
  /** Play the turn before the send returns, as a fast worker may. */
  eager: boolean
  seq: number
}

const fresh = (): State => ({
  threads: [],
  messages: {},
  script: [],
  sent: [],
  ratings: [],
  created: [],
  failEvents: false,
  failSend: false,
  busy: false,
  eager: false,
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

export function addThread(
  type: ThreadType = 'THREAD_TYPE_GENERAL',
  createdAt = '2026-11-13T09:00:00Z',
  extra: Partial<Pick<Thread, 'title' | 'pin'>> = {},
): Thread {
  const t: Thread = { id: nextId(), type, title: '', pin: {}, created_at: createdAt, ...extra }
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
  { event: 'status', data: { text: 'ищу, к кому обратиться' } },
  { event: 'delta', data: { text: text.slice(0, 5) } },
  { event: 'delta', data: { text: text.slice(5) } },
  {
    event: 'block',
    data: { block: { type: 'person', person_id: 'p-igor', name: 'Игорь Волков', reason: 'деплой', contact: '@igor' } },
  },
  { event: 'done', data: { text } },
]

/** Sends an event to the open stream of a thread, outside any turn (e.g. request_status). */
export function emit(threadId: string, e: ScriptEvent) {
  push(threadId, e)
}

function push(threadId: string, e: ScriptEvent) {
  streams.get(threadId)?.enqueue(encoder.encode(`event: ${e.event}\ndata: ${JSON.stringify(e.data)}\n\n`))
}

/** Plays the script as the turn jobId; events with their own job_id belong to another, older turn. */
function play(threadId: string, jobId: string) {
  const blocks: ChatMessage['blocks'] = []
  for (const e of chat.script) {
    const own = e.data.job_id === undefined
    const data: Record<string, unknown> = { ...e.data, job_id: e.data.job_id ?? jobId }
    if (own && e.event === 'block') blocks.push(e.data.block as ChatMessage['blocks'][number])
    if (own && e.event === 'done') data.message_id = addMessage(threadId, 'assistant', String(e.data.text), blocks).id
    push(threadId, { event: e.event, data })
  }
}

export const chatHandlers = [
  http.get(`${API}/v1/private/threads`, () => HttpResponse.json({ threads: chat.threads })),
  http.post(`${API}/v1/private/threads`, async ({ request }) => {
    const body = (await request.json()) as { type: ThreadType; title?: string; pin?: Thread['pin'] }
    chat.created.push(body)
    const extra = { title: body.title ?? '', pin: body.pin ?? {} }
    return HttpResponse.json({ thread: addThread(body.type, '2026-11-13T10:00:00Z', extra) })
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
    if (chat.busy)
      return HttpResponse.json({ code: 9, message: 'the previous answer is still being prepared', details: [] }, { status: 409 })
    const id = String(params.id)
    const body = (await request.json()) as { content: string; mode: string }
    chat.sent.push({ threadId: id, content: body.content, mode: body.mode })
    const msg = addMessage(id, 'user', body.content)
    const jobId = `job-${chat.sent.length}`
    if (chat.eager) {
      play(id, jobId)
    } else {
      const scheduled = generation
      setTimeout(() => scheduled === generation && play(id, jobId), 10)
    }
    return HttpResponse.json({ message_id: msg.id, job_id: jobId }, { status: 202 })
  }),
  http.post(`${API}/v1/private/messages/:id/feedback`, async ({ params, request }) => {
    const body = (await request.json()) as { value: string }
    chat.ratings.push({ messageId: String(params.id), value: body.value })
    return HttpResponse.json({})
  }),
]

/** An answer in the shape the live model writes: inline and display math, a GFM table and a Go block. */
export const richAnswer = [
  'Перцентиль $x_p$ считаем так:',
  '',
  '$$\\frac{a}{b}$$',
  '',
  '| Метрика | Порог |',
  '| --- | --- |',
  '| p99 | 200 мс |',
  '',
  '```go',
  'func main() { fmt.Println("ok") }',
  '```',
  '',
  'Команда `make deploy`, подробности в [1]. ~~Старый способ~~ не работает.',
  '',
  '- [x] прочитать README',
  '- [ ] поднять стенд',
].join('\n')
