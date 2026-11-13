import { describe, expect, it } from 'vitest'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderApp } from '../../test/render'
import { panels, today } from '../../test/panels'

const panel = () => screen.findByRole('region', { name: 'Сегодня' })

describe('today panel', () => {
  it('shows the stage, progress, next steps and blockers from the API', async () => {
    renderApp('/today')
    const p = await panel()
    expect(await within(p).findByText('Поднять локальное окружение')).toBeInTheDocument()
    expect(within(p).getByText('День 3 · неделя 1 · этап «1 неделя» · 4 из 17 шагов')).toBeInTheDocument()

    const steps = within(p).getByRole('list', { name: 'Следующие шаги' })
    expect(within(steps).getAllByRole('listitem')).toHaveLength(2)
    expect(within(steps).getByText('В работе')).toBeInTheDocument()
    expect(within(steps).getByText('Запусти сервис по README команды')).toBeInTheDocument()

    const blockers = within(p).getByRole('list', { name: 'Что мешает' })
    expect(within(blockers).getByText('Доступ к репозиторию messenger-api')).toBeInTheDocument()
    expect(within(blockers).getByText('Ждёт согласования руководителя')).toBeInTheDocument()
    expect(within(blockers).getByText('Заявка ещё не отправлена')).toBeInTheDocument()
  })

  it('puts a question about a step into the chat', async () => {
    const user = userEvent.setup()
    renderApp('/today')
    await user.click(await screen.findByRole('button', { name: /Поднять локальное окружение/ }))
    expect(screen.getByLabelText('Сообщение для buddy')).toHaveValue(
      'Помоги со шагом «Поднять локальное окружение»: с чего начать?',
    )
  })

  it('shows the live step count on the orb and the day in the greeting', async () => {
    renderApp('/')
    const nav = await screen.findByRole('navigation', { name: 'Разделы' })
    expect(await within(nav).findByRole('link', { name: /Сегодня 2 шага/ })).toBeInTheDocument()
    expect(screen.getByText('День 3 · неделя 1')).toBeInTheDocument()
  })

  it('says when there is nothing to do right now', async () => {
    panels.today = { ...today, next_steps: [], blockers: [], done: 17 }
    renderApp('/today')
    const p = await panel()
    expect(await within(p).findByText('На сейчас всё сделано. Загляни в план или спроси buddy, что дальше.')).toBeInTheDocument()
    expect(within(p).queryByRole('list', { name: 'Что мешает' })).not.toBeInTheDocument()
  })

  it('shows a failure and loads again on retry', async () => {
    const user = userEvent.setup()
    panels.fail.today = true
    renderApp('/today')
    const p = await panel()
    expect(await within(p).findByRole('alert')).toHaveTextContent('Не удалось загрузить')
    panels.fail.today = false
    await user.click(within(p).getByRole('button', { name: 'Повторить' }))
    expect(await within(p).findByText('Поднять локальное окружение')).toBeInTheDocument()
  })

  it('shows a loading state before the data arrives', async () => {
    renderApp('/today')
    const p = await panel()
    expect(within(p).getByRole('status')).toHaveTextContent('Загружаем')
  })
})
