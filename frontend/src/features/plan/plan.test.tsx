import { describe, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { addThread, chat } from '../../test/chat'
import { panels } from '../../test/panels'
import { renderApp } from '../../test/render'

const panel = () => screen.findByRole('region', { name: 'План адаптации' })

describe('plan panel', () => {
  it('lists the plan by stage with statuses and progress', async () => {
    renderApp('/plan')
    const p = await panel()
    const w1 = await within(p).findByRole('list', { name: '1 неделя' })
    expect(within(w1).getAllByRole('listitem')).toHaveLength(3)
    expect(within(w1).getByRole('button', { name: /Запросить доступы.*В работе/ })).toBeInTheDocument()
    expect(within(w1).getByRole('button', { name: /Настроить avito-cli.*Заблокирован/ })).toBeInTheDocument()
    expect(within(p).getByRole('list', { name: '2 неделя' })).toBeInTheDocument()
    expect(within(p).getByText('Сделано 1 из 4')).toBeInTheDocument()
  })

  it('shows the live progress on the orb', async () => {
    renderApp('/')
    const nav = await screen.findByRole('navigation', { name: 'Разделы' })
    expect(await within(nav).findByRole('link', { name: /План 1 из 4/ })).toBeInTheDocument()
  })

  it('opens an item with its practice, note and what blocks it', async () => {
    const user = userEvent.setup()
    renderApp('/plan')
    const p = await panel()
    await user.click(await within(p).findByRole('button', { name: /Настроить avito-cli/ }))
    const card = within(p).getByRole('group', { name: 'Настроить avito-cli' })
    expect(card).toHaveTextContent('Ждёт: Запросить доступы')

    await user.click(within(p).getByRole('button', { name: /Запросить доступы/ }))
    const access = within(p).getByRole('group', { name: 'Запросить доступы' })
    expect(access).toHaveTextContent('Отправь заявки на Jira, Bitbucket и VPN через buddy')
    expect(access).toHaveTextContent('VPN ещё на согласовании')
    expect(within(p).queryByRole('group', { name: 'Настроить avito-cli' })).not.toBeInTheDocument()
  })

  it('marks an item done and updates the progress', async () => {
    const user = userEvent.setup()
    renderApp('/plan')
    const p = await panel()
    await user.click(await within(p).findByRole('button', { name: /Запросить доступы/ }))
    await user.click(within(p).getByRole('button', { name: 'Отметить сделанным' }))
    expect(await within(p).findByText('Сделано 2 из 4')).toBeInTheDocument()
    expect(panels.patched).toEqual([{ id: 'w1-access', status: 'PLAN_ITEM_STATUS_DONE' }])
    expect(within(p).getByRole('button', { name: /Запросить доступы.*Готово/ })).toBeInTheDocument()
  })

  it('says when the status could not be saved', async () => {
    const user = userEvent.setup()
    panels.fail.patch = true
    renderApp('/plan')
    const p = await panel()
    await user.click(await within(p).findByRole('button', { name: /Пройти первое ревью/ }))
    await user.click(within(p).getByRole('button', { name: 'Начать' }))
    expect(await within(p).findByRole('alert')).toHaveTextContent('Статус не сохранился')
  })

  it('moves the chat to the plan thread with a question about the item', async () => {
    const user = userEvent.setup()
    addThread('THREAD_TYPE_GENERAL')
    renderApp('/plan')
    const p = await panel()
    await user.click(await within(p).findByRole('button', { name: /Запросить доступы/ }))
    await user.click(within(p).getByRole('button', { name: 'Обсудить в треде плана' }))

    await waitFor(() => expect(chat.threads.some((t) => t.type === 'THREAD_TYPE_PLAN')).toBe(true))
    const plan = chat.threads.find((t) => t.type === 'THREAD_TYPE_PLAN')
    await waitFor(() => expect(screen.getByLabelText('Тред')).toHaveValue(plan?.id))
    expect(screen.getByLabelText('Сообщение для buddy')).toHaveValue('Расскажи про пункт плана «Запросить доступы»: что сделать?')
  })

  it('reuses the existing plan thread', async () => {
    const user = userEvent.setup()
    addThread('THREAD_TYPE_GENERAL', '2026-11-13T09:00:00Z')
    const existing = addThread('THREAD_TYPE_PLAN', '2026-11-12T09:00:00Z')
    renderApp('/plan')
    const p = await panel()
    await user.click(await within(p).findByRole('button', { name: /Запросить доступы/ }))
    await user.click(within(p).getByRole('button', { name: 'Обсудить в треде плана' }))
    await waitFor(() => expect(screen.getByLabelText('Тред')).toHaveValue(existing.id))
    expect(chat.threads.filter((t) => t.type === 'THREAD_TYPE_PLAN')).toHaveLength(1)
  })

  it('shows a failure with retry and an empty plan', async () => {
    const user = userEvent.setup()
    panels.fail.plan = true
    renderApp('/plan')
    const p = await panel()
    expect(await within(p).findByRole('alert')).toHaveTextContent('Не удалось загрузить')
    panels.fail.plan = false
    panels.plan = { items: [], done: 0, total: 0 }
    await user.click(within(p).getByRole('button', { name: 'Повторить' }))
    expect(await within(p).findByText('План пока пуст: buddy соберёт его вместе с наставником.')).toBeInTheDocument()
  })
})
