import { describe, expect, it } from 'vitest'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderApp } from '../../test/render'
import { sections } from '../nav/sections'

const renderAt = (path: string) => renderApp(path).router

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

  it('takes a question from the info panel into the input', async () => {
    const user = userEvent.setup()
    renderAt('/work')
    await user.click(await screen.findByRole('button', { name: /Спросить buddy/ }))
    const input = screen.getByLabelText('Сообщение для buddy')
    expect(input).toHaveValue('В каком статусе мои заявки на доступы?')
    expect(input).toHaveFocus()
  })
})
