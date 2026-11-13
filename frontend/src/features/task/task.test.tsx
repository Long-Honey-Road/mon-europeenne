import { describe, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { addThread, chat } from '../../test/chat'
import { renderApp } from '../../test/render'
import { pendingRequest, requests } from '../../test/requests'

const taskThread = () =>
  addThread('THREAD_TYPE_TASK', '2026-11-13T11:00:00Z', {
    title: 'MSG-142 · Счётчик непрочитанных',
    pin: { task: { key: 'MSG-142', title: 'Счётчик непрочитанных', repo: 'messenger-api' } },
  })

/**
 * These tests switch the chat to a new thread mid-test. Aborting a live fake SSE stream makes undici assert in Node,
 * and the stream does not matter here, so the event endpoint is down for them.
 */
const withoutStreams = () => {
  chat.failEvents = true
}
const chatReady = () => waitFor(() => expect(screen.getByLabelText('Тред')).not.toBeDisabled())

const screenOf = (key: string) => screen.findByRole('region', { name: `Задача ${key}` })

describe('starting a work task', () => {
  it('starts a task from «Сегодня» and opens its screen with the thread in the chat', async () => {
    const user = userEvent.setup()
    withoutStreams()
    addThread('THREAD_TYPE_GENERAL')
    const { router } = renderApp('/today')
    const panel = await screen.findByRole('region', { name: 'Сегодня' })
    await chatReady()
    await user.click(within(panel).getByRole('button', { name: 'Разобрать рабочую задачу' }))

    const form = within(panel).getByRole('form', { name: 'Новая рабочая задача' })
    const start = within(form).getByRole('button', { name: 'Начать разбор' })
    expect(start).toBeDisabled()
    await user.type(within(form).getByLabelText(/Ключ задачи/), 'MSG-142')
    await user.type(within(form).getByLabelText(/Название/), 'Счётчик непрочитанных')
    expect(within(form).getByLabelText(/Репозиторий/)).toHaveValue('messenger-api')
    await user.click(start)

    const task = await screenOf('MSG-142')
    expect(task).toHaveTextContent('Счётчик непрочитанных')
    expect(chat.created).toEqual([
      {
        type: 'THREAD_TYPE_TASK',
        title: 'MSG-142 · Счётчик непрочитанных',
        pin: { task: { key: 'MSG-142', title: 'Счётчик непрочитанных', repo: 'messenger-api' } },
      },
    ])
    const created = chat.threads.find((t) => t.type === 'THREAD_TYPE_TASK')
    expect(router.state.location.pathname).toBe(`/task/${created?.id}`)
    await waitFor(() => expect(screen.getByLabelText('Тред')).toHaveValue(created?.id))
  })

  it('upper-cases the key and keeps an optional title out', async () => {
    const user = userEvent.setup()
    withoutStreams()
    addThread('THREAD_TYPE_GENERAL')
    renderApp('/work')
    const panel = await screen.findByRole('region', { name: 'Задачи и заявки' })
    await chatReady()
    await user.click(within(panel).getByRole('button', { name: 'Разобрать рабочую задачу' }))
    const form = within(panel).getByRole('form', { name: 'Новая рабочая задача' })
    await user.type(within(form).getByLabelText(/Ключ задачи/), ' msg-7 ')
    await user.click(within(form).getByRole('button', { name: 'Начать разбор' }))
    await screenOf('MSG-7')
    expect(chat.created[0]).toEqual({
      type: 'THREAD_TYPE_TASK',
      title: 'MSG-7',
      pin: { task: { key: 'MSG-7', repo: 'messenger-api' } },
    })
  })
})

describe('task screen', () => {
  it('walks through the four steps and asks buddy about each', async () => {
    const user = userEvent.setup()
    const t = taskThread()
    renderApp(`/task/${t.id}`)
    const task = await screenOf('MSG-142')
    const steps = within(task).getByRole('list', { name: 'Шаги разбора' })
    expect(within(steps).getAllByRole('listitem').map((li) => within(li).getByRole('heading').textContent)).toEqual([
      'Понять задачу',
      'Разобраться в сервисе',
      'Как принято в команде',
      'Проверить и отдать',
    ])
    await waitFor(() => expect(screen.getByLabelText('Тред')).toHaveValue(t.id))

    const second = within(steps).getAllByRole('listitem')[1]!
    await user.click(within(second).getByRole('button', { name: 'Спросить buddy' }))
    expect(screen.getByLabelText('Сообщение для buddy')).toHaveValue(
      'Шаг 2 из 4, разобраться в сервисе messenger-api: где README, как запустить локально и поднять dev-окружение?',
    )
  })

  it('marks steps passed and remembers it', async () => {
    const user = userEvent.setup()
    const t = taskThread()
    renderApp(`/task/${t.id}`)
    const task = await screenOf('MSG-142')
    expect(within(task).getByText('Пройдено 0 из 4')).toBeInTheDocument()
    const first = within(within(task).getByRole('list', { name: 'Шаги разбора' })).getAllByRole('listitem')[0]!
    await user.click(within(first).getByRole('button', { name: 'Отметить пройденным' }))
    expect(within(task).getByText('Пройдено 1 из 4')).toBeInTheDocument()
    expect(first).toHaveAttribute('data-done', 'true')

    await user.click(within(first).getByRole('button', { name: 'Вернуть' }))
    expect(within(task).getByText('Пройдено 0 из 4')).toBeInTheDocument()
  })

  it('says when the task is not found', async () => {
    renderApp('/task/missing')
    expect(await screen.findByText('Задача не найдена: возможно, тред удалён или открыт другим пользователем.')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Все разделы/ })).toHaveAttribute('href', '/')
  })
})

describe('«Задачи» orb', () => {
  it('lists task threads and the user’s requests', async () => {
    const t = taskThread()
    requests.requests = [pendingRequest('th-1', 'access.vpn', { justification: 'Работа со стендами' })]
    renderApp('/work')
    const panel = await screen.findByRole('region', { name: 'Задачи и заявки' })
    const link = await within(panel).findByRole('link', { name: /MSG-142/ })
    expect(link).toHaveAttribute('href', `/task/${t.id}`)
    const list = await within(panel).findByRole('list', { name: 'Заявки' })
    expect(list).toHaveTextContent('Доступ к корпоративному VPN')
    expect(list).toHaveTextContent('Ждёт согласования')
  })

  it('shows the live counts on the orb', async () => {
    taskThread()
    requests.requests = [pendingRequest('th-1', 'access.vpn', {})]
    renderApp('/')
    const nav = await screen.findByRole('navigation', { name: 'Разделы' })
    expect(await within(nav).findByRole('link', { name: /Задачи 1 задача · 1 ждёт/ })).toBeInTheDocument()
  })

  it('says when there are no tasks and no requests', async () => {
    renderApp('/work')
    const panel = await screen.findByRole('region', { name: 'Задачи и заявки' })
    expect(await within(panel).findByText('Рабочих задач пока нет.')).toBeInTheDocument()
    expect(await within(panel).findByText('Заявок пока нет: их готовит buddy, когда нужен доступ.')).toBeInTheDocument()
  })
})
