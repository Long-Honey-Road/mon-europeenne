import { describe, expect, it } from 'vitest'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderApp } from '../../test/render'
import { panels } from '../../test/panels'

const panel = () => screen.findByRole('region', { name: 'Команда' })

describe('team panel', () => {
  it('lists the own team by default and others on demand', async () => {
    const user = userEvent.setup()
    renderApp('/team')
    const p = await panel()
    expect(await within(p).findByText('Анна Белова')).toBeInTheDocument()
    expect(within(p).getByText('Игорь Волков')).toBeInTheDocument()
    expect(within(p).queryByText('Артём Фёдоров')).not.toBeInTheDocument()

    await user.click(within(p).getByRole('radio', { name: 'Все' }))
    expect(within(p).getByText('Артём Фёдоров')).toBeInTheDocument()
    expect(within(p).getByText('другая команда')).toBeInTheDocument()
  })

  it('opens a person and asks buddy about them', async () => {
    const user = userEvent.setup()
    renderApp('/team')
    const p = await panel()
    const anna = await within(p).findByRole('button', { name: /Анна Белова/ })
    expect(anna).toHaveAttribute('aria-expanded', 'false')
    await user.click(anna)
    expect(anna).toHaveAttribute('aria-expanded', 'true')
    expect(within(p).getByText('архитектура сервиса, первая задача')).toBeInTheDocument()
    expect(within(p).getByText('@anna.belova')).toBeInTheDocument()

    await user.click(within(p).getByRole('button', { name: 'Спросить buddy про человека: Анна Белова' }))
    expect(screen.getByLabelText('Сообщение для buddy')).toHaveValue(
      'Анна Белова — с какими вопросами идти к этому человеку?',
    )
  })

  it('shows the live head count on the orb', async () => {
    renderApp('/')
    const nav = await screen.findByRole('navigation', { name: 'Разделы' })
    expect(await within(nav).findByRole('link', { name: /Команда 2 человека/ })).toBeInTheDocument()
  })

  it('says when nobody is found', async () => {
    panels.people = { people: [] }
    renderApp('/team')
    const p = await panel()
    expect(await within(p).findByText('В справочнике пока никого нет.')).toBeInTheDocument()
  })

  it('shows a failure and loads again on retry', async () => {
    const user = userEvent.setup()
    panels.fail.people = true
    renderApp('/team')
    const p = await panel()
    expect(await within(p).findByRole('alert')).toHaveTextContent('Не удалось загрузить')
    panels.fail.people = false
    await user.click(within(p).getByRole('button', { name: 'Повторить' }))
    expect(await within(p).findByText('Анна Белова')).toBeInTheDocument()
  })
})
