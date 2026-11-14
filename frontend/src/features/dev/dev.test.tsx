import { describe, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { renderApp } from '../../test/render'
import { server } from '../../test/server'
import { API } from '../../test/api'

type Sent = { path: string; body: unknown }

function devApi() {
  const sent: Sent[] = []
  const messages = [
    { id: 'm1', role: 'MESSAGE_ROLE_USER', text: 'что с доступами?', focus: { kind: 'FOCUS_KIND_PLAN_ITEM', id: 'w1-access' } },
    { id: 'm2', role: 'MESSAGE_ROLE_ASSISTANT', text: 'Ждём одобрения', focus: { kind: 'FOCUS_KIND_PLAN_ITEM', id: 'w1-access' } },
    { id: 'm3', role: 'MESSAGE_ROLE_ASSISTANT', text: 'Про SSH', focus: { kind: 'FOCUS_KIND_PLAN_ITEM', id: 'w1-ssh' } },
  ]
  const record = { id: 'v1', title: 'Доступы', summary: 'Jira первой', tags: [], version: 2, versions: [{ id: 'v1', title: 'Доступы', summary: 'x', tags: [], version: 1 }] }
  const record2 = { ...record, versions: undefined }
  const keep = async (request: Request, path: string) => {
    const body = request.method === 'GET' || request.method === 'DELETE' ? undefined : await request.json().catch(() => undefined)
    sent.push({ path: `${request.method} ${path}`, body })
  }
  server.use(
    http.post(`${API}/v1/private/threads`, async ({ request }) => {
      const body = (await request.json()) as { type: string }
      return HttpResponse.json({ thread: { id: body.type === 'THREAD_TYPE_VAULT' ? 'tv' : 'tp' } })
    }),
    http.get(`${API}/v1/private/threads/:id/messages`, () => HttpResponse.json({ messages })),
    http.get(`${API}/v1/private/threads/:id/events`, () => {
      const enc = new TextEncoder()
      const stream = new ReadableStream<Uint8Array>({
        start(c) {
          c.enqueue(enc.encode(': ok\n\nevent: highlight\ndata: {"ids":["w1-access"]}\n\nevent: done\ndata: {}\n\n'))
          c.close()
        },
      })
      return new HttpResponse(stream, { headers: { 'Content-Type': 'text/event-stream' } })
    }),
    http.get(`${API}/v1/private/plan`, () =>
      HttpResponse.json({ items: [{ item_id: 'w1-access', title: 'Запросить доступы', status: 'ITEM_STATUS_DOING' }], done: 0, total: 1 }),
    ),
    http.get(`${API}/v1/private/vault`, () => HttpResponse.json({ items: [record2] })),
    http.get(`${API}/v1/private/vault/:id`, () => HttpResponse.json({ item: record })),
    http.get(`${API}/v1/private/pins`, () => HttpResponse.json({ pins: [{ id: 'p1', title: 'Порядок', text: 'Jira, VPN' }] })),
    http.post(`${API}/v1/private/threads/:id/messages`, async ({ request, params }) => {
      await keep(request, `/threads/${String(params.id)}/messages`)
      return HttpResponse.json({ message_id: 'm9', job_id: 'j1' }, { status: 202 })
    }),
    http.post(`${API}/v1/private/messages/:id/pin`, async ({ request, params }) => {
      await keep(request, `/messages/${String(params.id)}/pin`)
      return HttpResponse.json({ pin: {} })
    }),
    http.post(`${API}/v1/private/messages/:id/vault`, async ({ request, params }) => {
      await keep(request, `/messages/${String(params.id)}/vault`)
      return HttpResponse.json({ item: record2 })
    }),
    http.delete(`${API}/v1/private/pins/:id`, async ({ request, params }) => {
      await keep(request, `/pins/${String(params.id)}`)
      return HttpResponse.json({})
    }),
    http.post(`${API}/v1/private/vault/:id/restore`, async ({ request, params }) => {
      await keep(request, `/vault/${String(params.id)}/restore`)
      return HttpResponse.json({ item: record2 })
    }),
  )
  return sent
}

describe('dev spheres page', () => {
  it('sends with the selected sphere, shows its pins and pins an answer', async () => {
    const sent = devApi()
    renderApp('/dev/spheres')
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /w1-access · Запросить доступы/ }))
    expect(await screen.findByText(/Порядок/)).toBeInTheDocument()
    expect(screen.queryByText(/Про SSH/)).not.toBeInTheDocument()
    expect(await screen.findByText(/highlight/)).toBeInTheDocument()

    await user.type(screen.getByLabelText('Сообщение'), 'закрепи это')
    await user.click(screen.getByRole('button', { name: 'Отправить' }))
    await waitFor(() => expect(sent.some((s) => s.path === 'POST /threads/tp/messages')).toBe(true))
    expect(sent.find((s) => s.path === 'POST /threads/tp/messages')?.body).toMatchObject({
      content: 'закрепи это',
      focus: { kind: 'FOCUS_KIND_PLAN_ITEM', id: 'w1-access' },
    })

    await user.click(screen.getByRole('button', { name: 'Закрепить' }))
    await user.click(screen.getByRole('button', { name: 'В хранилище' }))
    await user.click(within(screen.getByRole('list', { name: 'Закрепы' })).getByRole('button', { name: 'удалить' }))
    await waitFor(() =>
      expect(sent.map((s) => s.path)).toEqual(
        expect.arrayContaining(['POST /messages/m2/pin', 'POST /messages/m2/vault', 'DELETE /pins/p1']),
      ),
    )
  })

  it('opens a vault record with its versions and restores one', async () => {
    const sent = devApi()
    renderApp('/dev/spheres')
    const user = userEvent.setup()
    await user.click(await screen.findByRole('tab', { name: 'Хранилище' }))
    await user.click(await screen.findByRole('button', { name: /Доступы · v2/ }))
    expect(await screen.findByText('Jira первой')).toBeInTheDocument()
    await user.click(within(screen.getByRole('list', { name: 'Версии' })).getByRole('button', { name: 'вернуть' }))
    await waitFor(() => expect(sent.find((s) => s.path === 'POST /vault/v1/restore')?.body).toEqual({ version: 1 }))
  })

  it('shows API errors and sends to the overview without a focus', async () => {
    devApi()
    server.use(http.post(`${API}/v1/private/threads/:id/messages`, () => HttpResponse.json({ message: 'занято' }, { status: 409 })))
    renderApp('/dev/spheres')
    const user = userEvent.setup()
    await user.type(await screen.findByLabelText('Сообщение'), 'привет')
    await user.click(screen.getByRole('button', { name: 'Отправить' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('занято')
  })

  it('sends a signed-out visitor to login', async () => {
    renderApp('/dev/spheres', { signedIn: false })
    expect(await screen.findByRole('button', { name: /войти/i })).toBeInTheDocument()
  })
})
