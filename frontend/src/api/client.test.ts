import { describe, expect, it, vi } from 'vitest'
import { HttpResponse, http } from 'msw'
import { API, TOKEN, intern, me, server } from '../test/api'
import { ApiError, createApi } from './client'

function apiWith(token: string | null, onUnauthorized = vi.fn()) {
  return { api: createApi({ baseUrl: API, getToken: () => token, onUnauthorized }), onUnauthorized }
}

describe('api client', () => {
  it('signs in and returns a validated session', async () => {
    const { api } = apiWith(null)
    await expect(api.login('intern', 'intern-demo')).resolves.toEqual({ token: TOKEN, user: intern })
  })

  it('rejects a wrong password as unauthorized without ending the session', async () => {
    const { api, onUnauthorized } = apiWith(null)
    const err = await api.login('intern', 'nope').catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ kind: 'unauthorized', status: 401, message: 'invalid login or password' })
    expect(onUnauthorized).not.toHaveBeenCalled()
  })

  it('sends the bearer token to private endpoints and maps null messages to undefined', async () => {
    const { api } = apiWith(TOKEN)
    await expect(api.me()).resolves.toEqual(me)
  })

  it('never sends the token to public endpoints', async () => {
    let auth: string | null = 'unset'
    server.use(
      http.post(`${API}/v1/public/auth/login`, ({ request }) => {
        auth = request.headers.get('Authorization')
        return HttpResponse.json({ token: TOKEN, user: intern })
      }),
    )
    await apiWith(TOKEN).api.login('intern', 'intern-demo')
    expect(auth).toBeNull()
  })

  it('reports a rejected session on a private endpoint', async () => {
    const { api, onUnauthorized } = apiWith('expired')
    await expect(api.me()).rejects.toMatchObject({ kind: 'unauthorized', status: 401 })
    expect(onUnauthorized).toHaveBeenCalledOnce()
  })

  it('turns a response that breaks the contract into an error', async () => {
    server.use(http.get(`${API}/v1/private/me`, () => HttpResponse.json({ user: { id: 1 } })))
    await expect(apiWith(TOKEN).api.me()).rejects.toMatchObject({ kind: 'invalid_response', status: 200 })
  })

  it('keeps the server message on other errors', async () => {
    server.use(
      http.get(`${API}/v1/private/me`, () =>
        HttpResponse.json({ code: 14, message: 'directory unavailable', details: [] }, { status: 503 }),
      ),
    )
    await expect(apiWith(TOKEN).api.me()).rejects.toMatchObject({
      kind: 'server',
      status: 503,
      message: 'directory unavailable',
    })
  })

  it('falls back to the status text when the error body is not a status', async () => {
    server.use(http.get(`${API}/v1/private/me`, () => new HttpResponse('oops', { status: 502, statusText: 'Bad Gateway' })))
    await expect(apiWith(TOKEN).api.me()).rejects.toMatchObject({ kind: 'server', status: 502, message: 'Bad Gateway' })
  })

  it('reports a network failure', async () => {
    server.use(http.get(`${API}/v1/private/me`, () => HttpResponse.error()))
    await expect(apiWith(TOKEN).api.me()).rejects.toMatchObject({ kind: 'network', status: 0 })
  })
})
