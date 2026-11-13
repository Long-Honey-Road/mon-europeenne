import { describe, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { addThread, chat, emit } from '../../test/chat'
import { panels } from '../../test/panels'
import { renderApp } from '../../test/render'
import { layoutPlan } from './layout'

const map = () => screen.findByRole('region', { name: 'Карта плана' })

describe('plan map', () => {
  it('draws every item as a node by stage, with links for dependencies', async () => {
    renderApp('/plan/map')
    const m = await map()
    expect(await within(m).findByRole('button', { name: 'Запросить доступы · В работе' })).toBeInTheDocument()
    expect(within(m).getByRole('button', { name: 'Настроить avito-cli · Заблокирован' })).toBeInTheDocument()
    expect(within(m).getByRole('button', { name: 'Пройти первое ревью · Впереди' })).toBeInTheDocument()
    expect(within(m).getByText('1 неделя')).toBeInTheDocument()
    expect(within(m).getByText('2 неделя')).toBeInTheDocument()
    // w1-cli ← w1-access, w2-review ← w1-cli and w1-team
    expect(m.querySelectorAll('[data-edge]')).toHaveLength(3)
    expect(m.querySelector('[data-edge="w1-access>w1-cli"]')).toHaveAttribute('data-done', 'false')
    expect(m.querySelector('[data-edge="w1-team>w2-review"]')).toHaveAttribute('data-done', 'true')
  })

  it('opens the plan thread in the chat', async () => {
    addThread('THREAD_TYPE_GENERAL')
    renderApp('/plan/map')
    await map()
    await waitFor(() => expect(chat.threads.some((t) => t.type === 'THREAD_TYPE_PLAN')).toBe(true))
    const plan = chat.threads.find((t) => t.type === 'THREAD_TYPE_PLAN')
    await waitFor(() => expect(screen.getByLabelText('Тред')).toHaveValue(plan?.id))
  })

  it('selects a node, shows its card and changes its status', async () => {
    const user = userEvent.setup()
    renderApp('/plan/map')
    const m = await map()
    const node = await within(m).findByRole('button', { name: /Пройти первое ревью/ })
    await user.click(node)
    expect(node).toHaveAttribute('aria-pressed', 'true')
    const card = within(m).getByRole('group', { name: 'Пройти первое ревью' })
    await user.click(within(card).getByRole('button', { name: 'Начать' }))
    expect(await within(m).findByRole('button', { name: 'Пройти первое ревью · В работе' })).toBeInTheDocument()
    expect(panels.patched).toEqual([{ id: 'w2-review', status: 'PLAN_ITEM_STATUS_DOING' }])

    await user.click(within(card).getByRole('button', { name: 'Закрыть карточку' }))
    expect(within(m).queryByRole('group', { name: 'Пройти первое ревью' })).not.toBeInTheDocument()
  })

  it('asks buddy about the selected item', async () => {
    const user = userEvent.setup()
    renderApp('/plan/map')
    const m = await map()
    await user.click(await within(m).findByRole('button', { name: /Запросить доступы/ }))
    await user.click(within(m).getByRole('button', { name: 'Обсудить в треде плана' }))
    expect(screen.getByLabelText('Сообщение для buddy')).toHaveValue(
      'Расскажи про пункт плана «Запросить доступы»: что сделать?',
    )
  })

  it('lights up the nodes buddy points at, with its note', async () => {
    const plan = addThread('THREAD_TYPE_PLAN')
    renderApp('/plan/map')
    const m = await map()
    const node = await within(m).findByRole('button', { name: /Пройти первое ревью/ })
    await waitFor(() => expect(screen.getByLabelText('Тред')).toHaveValue(plan.id))
    await waitFor(() => {
      emit(plan.id, { event: 'highlight', data: { kind: 'plan_item', ids: ['w2-review'], note: 'Начни отсюда' } })
      expect(node).toHaveAttribute('data-highlight', 'true')
    })
    expect(within(m).getByText('Начни отсюда')).toBeInTheDocument()
    expect(within(m).getByRole('button', { name: /Запросить доступы/ })).toHaveAttribute('data-highlight', 'false')

    // The older event shape with item_ids also works.
    emit(plan.id, { event: 'highlight', data: { item_ids: ['w1-team'] } })
    await waitFor(() =>
      expect(within(m).getByRole('button', { name: /Познакомиться с командой/ })).toHaveAttribute('data-highlight', 'true'),
    )
    expect(node).toHaveAttribute('data-highlight', 'false')
  })

  it('switches between the map and the list', async () => {
    const user = userEvent.setup()
    const { router } = renderApp('/plan')
    const p = await screen.findByRole('region', { name: 'План адаптации' })
    await user.click(await within(p).findByRole('link', { name: 'Открыть карту' }))
    await map()
    expect(router.state.location.pathname).toBe('/plan/map')
    await user.click(screen.getByRole('link', { name: 'Списком' }))
    expect(await screen.findByRole('region', { name: 'План адаптации' })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/plan')
  })

  it('shows a failure with retry', async () => {
    const user = userEvent.setup()
    panels.fail.plan = true
    renderApp('/plan/map')
    const m = await map()
    expect(await within(m).findByRole('alert')).toHaveTextContent('Не удалось загрузить')
    panels.fail.plan = false
    await user.click(within(m).getByRole('button', { name: 'Повторить' }))
    expect(await within(m).findByRole('button', { name: /Запросить доступы/ })).toBeInTheDocument()
  })
})

describe('plan map layout', () => {
  it('puts stages top to bottom and spreads items of a stage across', () => {
    const { nodes, bands, height } = layoutPlan(
      (panels.plan as { items: Parameters<typeof layoutPlan>[0] }).items,
    )
    expect(bands.map((b) => b.title)).toEqual(['1 неделя', '2 неделя'])
    const w1 = nodes.filter((n) => n.item.period === 'PLAN_PERIOD_W1')
    expect(new Set(w1.map((n) => Math.round(n.x)))).toHaveProperty('size', 3)
    for (const n of nodes) {
      expect(n.x).toBeGreaterThan(0)
      expect(n.x).toBeLessThan(100)
      expect(n.y).toBeGreaterThan(0)
      expect(n.y).toBeLessThan(height)
    }
    const review = nodes.find((n) => n.item.id === 'w2-review')
    expect(review?.y).toBeGreaterThan(Math.max(...w1.map((n) => n.y)))
  })

  it('wraps a crowded stage into rows on narrow screens', () => {
    const items = (panels.plan as { items: Parameters<typeof layoutPlan>[0] }).items
    const many = Array.from({ length: 7 }, (_, i) => ({ ...items[0]!, id: `w1-${i}`, map: { x: 0, y: i, group: 'g' } }))
    const wide = layoutPlan(many)
    const narrow = layoutPlan(many, 3)
    expect(narrow.bands[0]!.height).toBeGreaterThan(wide.bands[0]!.height)
    expect(new Set(narrow.nodes.map((n) => Math.round(n.x)))).toHaveProperty('size', 3)
    expect(narrow.nodes[6]!.y).toBeGreaterThan(narrow.nodes[3]!.y)
  })

  it('keeps items without a map position', () => {
    const { nodes } = layoutPlan([
      { ...(panels.plan as { items: Parameters<typeof layoutPlan>[0] }).items[0]!, map: undefined },
    ])
    expect(nodes).toHaveLength(1)
  })
})
