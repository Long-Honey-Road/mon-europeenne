import { describe, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderApp } from '../../test/render'
import { addMessage, addThread, answer, chat } from '../../test/chat'
import { threadLabel } from './ChatPanel'

const panel = () => screen.findByRole('complementary', { name: 'Чат с buddy' })

async function ask(text: string) {
  const user = userEvent.setup()
  const input = await screen.findByLabelText('Сообщение для buddy')
  await waitFor(() => expect(screen.getByLabelText('Тред')).not.toBeDisabled())
  await user.type(input, `${text}{Enter}`)
  return user
}

describe('chat threads', () => {
  it('opens the newest general thread with its history and blocks', async () => {
    const older = addThread('THREAD_TYPE_GENERAL', '2026-11-12T09:00:00Z')
    addMessage(older.id, 'user', 'старый вопрос')
    addThread('THREAD_TYPE_PLAN')
    const general = addThread('THREAD_TYPE_GENERAL')
    addMessage(general.id, 'user', 'Кто поможет с деплоем?')
    addMessage(general.id, 'assistant', 'Спроси **Игоря** [1].', [
      { type: 'sources', items: [{ ref: 1, source_id: 'doc-1', title: 'Стенды', url: 'https://wiki.test/stands' }] },
      { type: 'person', person_id: 'p-igor', name: 'Игорь Волков', role_title: 'Инфраструктура', reason: 'деплой', contact: '@igor', other_team: true },
    ])
    renderApp('/')

    const log = within(await panel())
    expect(await log.findByText('Игоря')).toContainHTML('strong')
    expect(log.queryByText('старый вопрос')).not.toBeInTheDocument()
    expect(log.getByRole('link', { name: 'Стенды' })).toHaveAttribute('href', 'https://wiki.test/stands')
    const person = log.getByRole('region', { name: 'Контакт: Игорь Волков' })
    expect(person).toHaveTextContent('@igor')
    expect(person).toHaveTextContent('другая команда')
    expect(screen.getByLabelText('Тред')).toHaveValue(general.id)
  })

  it('starts a general thread when there is none', async () => {
    renderApp('/')
    await waitFor(() => expect(chat.threads).toHaveLength(1))
    expect(chat.threads[0]?.type).toBe('THREAD_TYPE_GENERAL')
    expect(await within(await panel()).findByText(/Привет, Аня/)).toBeInTheDocument()
  })

  it('creates a thread of a chosen type and switches back', async () => {
    const general = addThread()
    addMessage(general.id, 'user', 'вопрос в общем')
    const user = userEvent.setup()
    renderApp('/')
    expect(await screen.findByText('вопрос в общем')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Новый' }))
    await user.click(within(screen.getByRole('list', { name: 'Новый тред' })).getByRole('button', { name: 'Заявки' }))
    await waitFor(() => expect(chat.threads[0]?.type).toBe('THREAD_TYPE_REQUEST'))
    await waitFor(() => expect(screen.queryByText('вопрос в общем')).not.toBeInTheDocument())
    await waitFor(() => expect(screen.getByLabelText('Тред')).toHaveValue(chat.threads[0]?.id))

    await user.selectOptions(screen.getByLabelText('Тред'), general.id)
    expect(await screen.findByText('вопрос в общем')).toBeInTheDocument()
  })

  it('labels threads by type and date', () => {
    expect(threadLabel({ id: '1', type: 'THREAD_TYPE_PLAN', title: '', created_at: 'nope' })).toBe('План')
    expect(threadLabel({ id: '1', type: 'THREAD_TYPE_GENERAL', title: 'Деплой', created_at: '2026-11-13T09:00:00Z' })).toMatch(/^Деплой · 13 нояб/)
  })
})

describe('chat turn', () => {
  it('streams the answer, then shows the saved one with its blocks', async () => {
    addThread()
    chat.script = answer('Спроси Игоря, он отвечает за стенды.')
    renderApp('/')

    await ask('Кто поможет с деплоем?')
    const log = within(await panel())
    expect(log.getByText('Кто поможет с деплоем?')).toBeInTheDocument()
    // The stream and then the saved answer: one copy in the end, with its block and rating buttons.
    await waitFor(() => expect(log.getAllByRole('button', { name: 'Полезно' })).toHaveLength(1))
    expect(log.getAllByText('Спроси Игоря, он отвечает за стенды.')).toHaveLength(1)
    expect(log.getByRole('region', { name: 'Контакт: Игорь Волков' })).toBeInTheDocument()
    expect(screen.getByLabelText('Сообщение для buddy')).toHaveValue('')
    expect(chat.sent).toEqual([{ threadId: chat.threads[0]?.id, content: 'Кто поможет с деплоем?', mode: 'ANSWER_MODE_BRIEF' }])
  })

  it('shows what buddy is doing while the answer is not ready', async () => {
    addThread()
    chat.script = [{ event: 'status', data: { text: 'ищу в документах' } }]
    renderApp('/')

    await ask('Как поднять dev?')
    expect(await screen.findByText('buddy ищу в документах…')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Отправить' })).toBeDisabled()
    expect(screen.getByLabelText('Сообщение для buddy')).toHaveAttribute('placeholder', 'buddy отвечает…')
  })

  it('drops streamed text on reset and keeps the rest', async () => {
    addThread()
    chat.script = [
      { event: 'delta', data: { text: 'черновик' } },
      { event: 'reset', data: {} },
      { event: 'delta', data: { text: 'итог' } },
      { event: 'block', data: { block: { type: 'not_found', topic: 'dev', escalation: [{ level: 1, role: 'mentor', name: 'Анна' }] } } },
      { event: 'block', data: { block: { type: 'mystery' } } },
      { event: 'block', data: { block: 'not a block' } },
    ]
    renderApp('/')

    await ask('Как поднять dev?')
    const notFound = await screen.findByRole('region', { name: 'Не нашёл ответа' })
    expect(notFound).toHaveTextContent('Анна · наставник')
    expect(screen.getByText('итог')).toBeInTheDocument()
    expect(screen.queryByText('черновик')).not.toBeInTheDocument()
    expect(screen.getByRole('region', { name: 'Блок mystery' })).toBeInTheDocument()
  })

  it('shows a prepared request as a card', async () => {
    addThread('THREAD_TYPE_REQUEST')
    chat.script = [
      {
        event: 'block',
        data: {
          block: {
            type: 'action',
            action: {
              kind: 'request',
              type_key: 'access.vpn',
              title: 'Доступ к VPN',
              fields: { justification: 'работа из дома' },
              approvers: [{ role: 'devops', name: 'Артём' }, { role: 'manager' }],
            },
          },
        },
      },
      { event: 'done', data: { text: 'Проверь заявку.' } },
    ]
    renderApp('/')

    await ask('Нужен VPN')
    const card = await screen.findByRole('region', { name: 'Заявка: Доступ к VPN' })
    expect(card).toHaveTextContent('работа из дома')
    expect(card).toHaveTextContent('Согласует: Артём, руководитель')
  })

  it('shows a failed turn with whom to ask', async () => {
    addThread()
    chat.script = [
      { event: 'error', data: { code: 'model_unavailable', message: 'Модель недоступна', escalation: [{ level: 1, role: 'mentor' }] } },
    ]
    renderApp('/')

    await ask('привет')
    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('Модель недоступна')
    expect(alert).toHaveTextContent('наставник')
    expect(screen.getByRole('button', { name: 'Отправить' })).toBeDisabled()
    await userEvent.setup().type(screen.getByLabelText('Сообщение для buddy'), 'ещё раз')
    expect(screen.getByRole('button', { name: 'Отправить' })).toBeEnabled()
  })

  it('says so when the message cannot be sent', async () => {
    addThread()
    chat.failSend = true
    renderApp('/')
    await ask('привет')
    expect(await screen.findByRole('alert')).toHaveTextContent('Сообщение не отправилось')
  })

  it('does not send without the event stream', async () => {
    addThread()
    chat.failEvents = true
    renderApp('/')
    await ask('привет')
    expect(await screen.findByRole('alert')).toHaveTextContent('Нет связи с сервером')
    expect(chat.sent).toHaveLength(0)
  })
})

describe('answer mode and feedback', () => {
  it('sends in the detailed mode when chosen', async () => {
    addThread()
    chat.script = answer('Подробный ответ про стенды.')
    const user = userEvent.setup()
    renderApp('/')
    await user.click(await screen.findByRole('button', { name: 'Подробно' }))
    expect(screen.getByRole('button', { name: 'Подробно' })).toHaveAttribute('aria-pressed', 'true')

    await ask('Как устроены стенды?')
    await waitFor(() => expect(chat.sent[0]?.mode).toBe('ANSWER_MODE_DETAILED'))
  })

  it('asks the previous question again in detail', async () => {
    const t = addThread()
    addMessage(t.id, 'user', 'Кто поможет с деплоем?')
    addMessage(t.id, 'assistant', 'Игорь.')
    chat.script = answer('Игорь Волков отвечает за стенды и деплой.')
    const user = userEvent.setup()
    renderApp('/')

    await user.click(await within(await panel()).findByRole('button', { name: 'Подробнее' }))
    await waitFor(() => expect(chat.sent).toEqual([{ threadId: t.id, content: 'Кто поможет с деплоем?', mode: 'ANSWER_MODE_DETAILED' }]))
    expect(await screen.findByText('Игорь Волков отвечает за стенды и деплой.')).toBeInTheDocument()
  })

  it('rates an answer', async () => {
    const t = addThread()
    addMessage(t.id, 'user', 'вопрос')
    const reply = addMessage(t.id, 'assistant', 'ответ')
    const user = userEvent.setup()
    renderApp('/')

    const down = await screen.findByRole('button', { name: 'Не помогло' })
    await user.click(down)
    expect(down).toHaveAttribute('aria-pressed', 'true')
    await waitFor(() => expect(chat.ratings).toEqual([{ messageId: reply.id, value: 'RATING_DOWN' }]))
  })
})
