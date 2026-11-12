import { describe, expect, it } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RouterProvider, createMemoryHistory } from '@tanstack/react-router'
import { createAppRouter } from '../../app/router'
import { sections } from '../nav/sections'

function renderAt(path: string) {
  const router = createAppRouter(createMemoryHistory({ initialEntries: [path] }))
  render(<RouterProvider router={router} />)
  return router
}

const orbs = () => screen.findByRole('navigation', { name: 'Разделы' })

describe('scene', () => {
  it('shows the greeting and the four orbs as a logo cluster', async () => {
    renderAt('/')
    const nav = await orbs()
    expect(screen.getByRole('heading', { name: /Привет/ })).toBeInTheDocument()
    for (const s of sections) {
      const orb = within(nav).getByRole('link', { name: new RegExp(s.name) })
      expect(orb).toHaveAttribute('href', s.path)
      expect(orb).toHaveAttribute('data-state', 'cluster')
    }
  })

  it('puts the chosen orb in focus and attaches its information', async () => {
    const user = userEvent.setup()
    const router = renderAt('/')
    const nav = await orbs()
    await user.click(within(nav).getByRole('link', { name: /План/ }))

    expect(await screen.findByRole('heading', { name: 'План адаптации' })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/plan')
    expect(within(nav).getByRole('link', { name: /План/ })).toHaveAttribute('data-state', 'main')
    expect(within(nav).getByRole('link', { name: /Команда/ })).toHaveAttribute('data-state', 'mini')
    expect(screen.getByText('Рабочие инструменты')).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: /Привет/ })).not.toBeInTheDocument()
  })

  it('switches focus to another orb', async () => {
    const user = userEvent.setup()
    renderAt('/plan')
    const nav = await orbs()
    await user.click(within(nav).getByRole('link', { name: /Команда/ }))
    expect(await screen.findByRole('heading', { name: 'Команда' })).toBeInTheDocument()
    expect(within(nav).getByRole('link', { name: /План/ })).toHaveAttribute('data-state', 'mini')
  })

  it('returns to the cluster by link and by Escape', async () => {
    const user = userEvent.setup()
    const router = renderAt('/today')
    await user.click(await screen.findByRole('link', { name: /Все разделы/ }))
    expect(await screen.findByRole('heading', { name: /Привет/ })).toBeInTheDocument()

    await user.click(screen.getByRole('link', { name: /Задачи/ }))
    await screen.findByRole('heading', { name: 'Задачи и заявки' })
    await user.keyboard('{Escape}')
    expect(await screen.findByRole('heading', { name: /Привет/ })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/')
  })
})

describe('chat', () => {
  it('stays on screen and shows the section in focus as context', async () => {
    renderAt('/team')
    const chat = await screen.findByRole('complementary', { name: 'Чат с buddy' })
    expect(within(chat).getByText('Контекст: Команда')).toBeInTheDocument()
  })

  it('sends a message and answers with the placeholder reply', async () => {
    const user = userEvent.setup()
    renderAt('/')
    const input = await screen.findByLabelText('Сообщение для buddy')
    const send = screen.getByRole('button', { name: 'Отправить' })
    expect(send).toBeDisabled()

    await user.type(input, 'Как оформить отгул?{Enter}')
    expect(screen.getByText('Как оформить отгул?')).toBeInTheDocument()
    expect(screen.getByText(/Модель ещё не подключена/)).toBeInTheDocument()
    expect(input).toHaveValue('')
  })

  it('ignores an empty message', async () => {
    const user = userEvent.setup()
    renderAt('/')
    const input = await screen.findByLabelText('Сообщение для buddy')
    await user.type(input, '   {Enter}')
    expect(screen.queryByText(/Модель ещё не подключена/)).not.toBeInTheDocument()
  })

  it('takes a question from the info panel into the input', async () => {
    const user = userEvent.setup()
    renderAt('/work')
    await user.click(await screen.findByRole('button', { name: /Спросить buddy/ }))
    const input = screen.getByLabelText('Сообщение для buddy')
    expect(input).toHaveValue('В каком статусе мои заявки на доступы?')
    expect(input).toHaveFocus()
  })
})
