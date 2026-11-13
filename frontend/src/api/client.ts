import createClient from 'openapi-fetch'
import type { z } from 'zod'
import type { paths } from './schema.gen'
import {
  emptySchema,
  getMeResponseSchema,
  listMessagesResponseSchema,
  listThreadsResponseSchema,
  loginResponseSchema,
  sendMessageResponseSchema,
  statusSchema,
  threadResponseSchema,
} from './schemas'
import type { ChatMessage, LoginResponse, Me, Thread, ThreadType } from './schemas'

export type AnswerMode = 'brief' | 'detailed'
export type Rating = 'up' | 'down'

export type ApiErrorKind = 'unauthorized' | 'network' | 'invalid_response' | 'server'

export class ApiError extends Error {
  readonly kind: ApiErrorKind
  readonly status: number

  constructor(kind: ApiErrorKind, status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.kind = kind
    this.status = status
  }
}

export type ApiOptions = {
  baseUrl: string
  getToken: () => string | null
  /** Called when the server rejects the session on a private endpoint. */
  onUnauthorized?: () => void
  fetch?: typeof fetch
}

export type Api = {
  login: (login: string, password: string) => Promise<LoginResponse>
  me: () => Promise<Me>
  threads: () => Promise<Thread[]>
  createThread: (type: ThreadType) => Promise<Thread>
  messages: (threadId: string) => Promise<ChatMessage[]>
  /** Queues an agent turn; the answer arrives over the thread's event stream. */
  sendMessage: (threadId: string, content: string, mode: AnswerMode) => Promise<{ message_id: string; job_id: string }>
  rateMessage: (messageId: string, value: Rating) => Promise<void>
}

type Result = { data?: unknown; error?: unknown; response: Response }

export function createApi({ baseUrl, getToken, onUnauthorized, fetch: fetchImpl }: ApiOptions): Api {
  const client = createClient<paths>({ baseUrl, fetch: fetchImpl ?? ((req) => globalThis.fetch(req)) })

  client.use({
    onRequest({ request, schemaPath }) {
      const token = getToken()
      if (token && schemaPath.startsWith('/v1/private/')) {
        request.headers.set('Authorization', `Bearer ${token}`)
      }
      return request
    },
  })

  async function call<T>(send: () => Promise<Result>, schema: z.ZodType<T>, isPrivate: boolean): Promise<T> {
    let result: Result
    try {
      result = await send()
    } catch {
      throw new ApiError('network', 0, 'network error')
    }
    const { data, error, response } = result
    if (!response.ok) {
      const status = statusSchema.safeParse(error)
      const message = status.success ? status.data.message : response.statusText
      if (response.status === 401) {
        if (isPrivate) onUnauthorized?.()
        throw new ApiError('unauthorized', 401, message)
      }
      throw new ApiError('server', response.status, message)
    }
    const parsed = schema.safeParse(data)
    if (!parsed.success) throw new ApiError('invalid_response', response.status, parsed.error.message)
    return parsed.data
  }

  return {
    login: (login, password) =>
      call(() => client.POST('/v1/public/auth/login', { body: { login, password } }), loginResponseSchema, false),
    me: () => call(() => client.GET('/v1/private/me'), getMeResponseSchema, true),
    threads: async () =>
      (await call(() => client.GET('/v1/private/threads'), listThreadsResponseSchema, true)).threads,
    createThread: async (type) =>
      (await call(() => client.POST('/v1/private/threads', { body: { type } }), threadResponseSchema, true)).thread,
    messages: async (threadId) =>
      (
        await call(
          () => client.GET('/v1/private/threads/{thread_id}/messages', { params: { path: { thread_id: threadId } } }),
          listMessagesResponseSchema,
          true,
        )
      ).messages,
    sendMessage: (threadId, content, mode) =>
      call(
        () =>
          client.POST('/v1/private/threads/{thread_id}/messages', {
            params: { path: { thread_id: threadId } },
            body: { content, mode: mode === 'detailed' ? 'ANSWER_MODE_DETAILED' : 'ANSWER_MODE_BRIEF' },
          }),
        sendMessageResponseSchema,
        true,
      ),
    rateMessage: async (messageId, value) => {
      await call(
        () =>
          client.POST('/v1/private/messages/{message_id}/feedback', {
            params: { path: { message_id: messageId } },
            body: { value: value === 'up' ? 'RATING_UP' : 'RATING_DOWN' },
          }),
        emptySchema,
        true,
      )
    },
  }
}
