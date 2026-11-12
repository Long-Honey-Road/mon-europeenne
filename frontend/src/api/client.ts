import createClient from 'openapi-fetch'
import type { z } from 'zod'
import type { paths } from './schema.gen'
import { getMeResponseSchema, loginResponseSchema, statusSchema } from './schemas'
import type { LoginResponse, Me } from './schemas'

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
  }
}
