import createClient from 'openapi-fetch'
import type { z } from 'zod'
import type { paths } from './schema.gen'
import {
  addInternResponseSchema,
  emptySchema,
  getMeResponseSchema,
  getPlanResponseSchema,
  listMessagesResponseSchema,
  listPeopleResponseSchema,
  listRequestTypesResponseSchema,
  listRequestsResponseSchema,
  requestResponseSchema,
  listThreadsResponseSchema,
  loginResponseSchema,
  sendMessageResponseSchema,
  statusSchema,
  threadResponseSchema,
  todaySchema,
  updatePlanItemResponseSchema,
} from './schemas'
import type {
  AddInternResponse,
  ChatMessage,
  LoginResponse,
  Me,
  Person,
  Plan,
  PlanItem,
  PlanItemStatus,
  RequestType,
  ServiceRequest,
  TaskPin,
  Thread,
  ThreadType,
  Today,
} from './schemas'

export type AnswerMode = 'brief' | 'detailed'

export type NewIntern = {
  display_name: string
  login: string
  direction: 'DIRECTION_BACKEND' | 'DIRECTION_FRONTEND' | 'DIRECTION_QA'
  mentor_id: string
  start_date: string
}
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
  today: () => Promise<Today>
  plan: () => Promise<Plan>
  updatePlanItem: (itemId: string, status: PlanItemStatus) => Promise<PlanItem>
  /** Directory, the caller's team first. */
  people: () => Promise<Person[]>
  /** Managers only: creates the intern's account; the one-time password is in this response and nowhere else. */
  addIntern: (intern: NewIntern) => Promise<AddInternResponse>
  requestTypes: () => Promise<RequestType[]>
  /** The caller's own requests, last updated first. */
  myRequests: () => Promise<ServiceRequest[]>
  /** Sends a request the user confirmed; its status changes arrive in `threadId` as request_status events. */
  submitRequest: (threadId: string, typeKey: string, fields: Record<string, unknown>) => Promise<ServiceRequest>
  threads: () => Promise<Thread[]>
  createThread: (type: ThreadType, opts?: { title?: string; task?: TaskPin }) => Promise<Thread>
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
    today: () => call(() => client.GET('/v1/private/today'), todaySchema, true),
    plan: () => call(() => client.GET('/v1/private/plan'), getPlanResponseSchema, true),
    updatePlanItem: async (itemId, status) =>
      (
        await call(
          () =>
            client.PATCH('/v1/private/plan/items/{item_id}', {
              params: { path: { item_id: itemId } },
              body: { item_id: itemId, status },
            }),
          updatePlanItemResponseSchema,
          true,
        )
      ).item,
    people: async () => (await call(() => client.GET('/v1/private/people'), listPeopleResponseSchema, true)).people,
    addIntern: (intern) =>
      call(() => client.POST('/v1/private/team/interns', { body: intern }), addInternResponseSchema, true),
    requestTypes: async () =>
      (await call(() => client.GET('/v1/private/request-types'), listRequestTypesResponseSchema, true)).types,
    myRequests: async () =>
      (await call(() => client.GET('/v1/private/requests'), listRequestsResponseSchema, true)).requests,
    submitRequest: async (threadId, typeKey, fields) =>
      (
        await call(
          () => // protobuf Struct comes out of the OpenAPI generator as an object without keys.
          client.POST('/v1/private/requests', {
            body: { thread_id: threadId, type_key: typeKey, fields: fields as Record<string, never> },
          }),
          requestResponseSchema,
          true,
        )
      ).request,
    threads: async () =>
      (await call(() => client.GET('/v1/private/threads'), listThreadsResponseSchema, true)).threads,
    createThread: async (type, opts = {}) =>
      (
        await call(
          () =>
            client.POST('/v1/private/threads', {
              body: {
                type,
                ...(opts.title ? { title: opts.title } : {}),
                // protobuf Struct comes out of the OpenAPI generator as an object without keys.
                ...(opts.task ? { pin: { task: opts.task } as unknown as Record<string, never> } : {}),
              },
            }),
          threadResponseSchema,
          true,
        )
      ).thread,
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
