import { describe, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Block } from '../../api/schemas'
import { addMessage, addThread, emit } from '../../test/chat'
import { renderApp } from '../../test/render'
import { pendingRequest, requests } from '../../test/requests'

const vpnAction: Block = {
  type: 'action',
  action: {
    kind: 'request',
    type_key: 'access.vpn',
    title: 'Доступ к корпоративному VPN',
    fields: { justification: 'Работа со стендами из дома' },
    approvers: [{ role: 'devops', name: 'Игорь Волков' }],
  },
}

/** A general thread whose last answer prepared a request. */
function threadWith(block: Block = vpnAction) {
  const t = addThread('THREAD_TYPE_GENERAL')
  addMessage(t.id, 'user', 'Нужен VPN')
  addMessage(t.id, 'assistant', 'Заполнил заявку, проверь и отправь.', [block])
  return t
}

const form = () => screen.findByRole('form', { name: /Заявка: Доступ к корпоративному VPN/ })

describe('request form', () => {
  it('shows the prepared request as an editable form with who approves', async () => {
    threadWith()
    renderApp('/')
    const f = await form()
    expect(await within(f).findByLabelText(/Обоснование/)).toHaveValue('Работа со стендами из дома')
    expect(f).toHaveTextContent('Согласует: Игорь Волков')
    expect(within(f).getByRole('button', { name: 'Отправить заявку' })).toBeEnabled()
  })

  it('renders choices from the schema and marks required fields', async () => {
    threadWith({
      type: 'action',
      action: {
        kind: 'request',
        type_key: 'access.jira_project',
        title: 'Доступ к проекту Jira',
        fields: { project_key: 'MSG', level: 'read', justification: 'Учебная задача MSG-142' },
        approvers: [{ role: 'object_owner' }],
      },
    })
    renderApp('/')
    const f = await screen.findByRole('form', { name: /Доступ к проекту Jira/ })
    const level = await within(f).findByLabelText(/Уровень/)
    expect(level).toHaveValue('read')
    expect(within(level).getAllByRole('option').map((o) => o.textContent)).toEqual(['read', 'write'])
    expect(within(f).getByLabelText(/Ключ проекта/)).toBeRequired()
    expect(within(f).getByLabelText('comment')).not.toBeRequired()
    expect(f).toHaveTextContent('Согласует: владелец ресурса')
  })

  it('checks the fields before sending', async () => {
    const user = userEvent.setup()
    threadWith()
    renderApp('/')
    const f = await form()
    const field = await within(f).findByLabelText(/Обоснование/)
    await user.clear(field)
    await user.type(field, 'коротко')
    await user.click(within(f).getByRole('button', { name: 'Отправить заявку' }))
    expect(within(f).getByRole('alert')).toHaveTextContent('Обоснование: не короче 10 символов')
    expect(field).toHaveAttribute('aria-invalid', 'true')
    expect(requests.submitted).toHaveLength(0)
  })

  it('sends the edited request and shows that it waits for approval', async () => {
    const user = userEvent.setup()
    const t = threadWith()
    renderApp('/')
    const f = await form()
    const field = await within(f).findByLabelText(/Обоснование/)
    await user.type(field, ' и дежурства')
    await user.click(within(f).getByRole('button', { name: 'Отправить заявку' }))

    const card = await screen.findByRole('region', { name: 'Заявка REQ-1: Доступ к корпоративному VPN' })
    expect(card).toHaveTextContent('Ждёт согласования')
    expect(card).toHaveTextContent('Игорь Волков')
    expect(card).toHaveTextContent('Ждём решения DevOps, обычно до суток')
    expect(requests.submitted).toEqual([
      { thread_id: t.id, type_key: 'access.vpn', fields: { justification: 'Работа со стендами из дома и дежурства' } },
    ])
    expect(screen.queryByRole('form', { name: /Доступ к корпоративному VPN/ })).not.toBeInTheDocument()
  })

  it('updates the status when the service decides', async () => {
    const user = userEvent.setup()
    const t = threadWith()
    renderApp('/')
    await user.click(await within(await form()).findByRole('button', { name: 'Отправить заявку' }))
    const card = await screen.findByRole('region', { name: /Заявка REQ-1/ })

    emit(t.id, {
      event: 'request_status',
      data: {
        request_id: 'REQ-1',
        status: 'rejected',
        approver: { role: 'devops', name: 'Игорь Волков' },
        next_step: 'Уточни обоснование и подай снова',
        reason: 'VPN выдаём после вводного инструктажа',
      },
    })
    await waitFor(() => expect(card).toHaveTextContent('Отклонено'))
    expect(card).toHaveTextContent('VPN выдаём после вводного инструктажа')
    expect(card).toHaveTextContent('Уточни обоснование и подай снова')

    emit(t.id, { event: 'request_status', data: { request_id: 'REQ-1', status: 'approved', next_step: '' } })
    await waitFor(() => expect(card).toHaveTextContent('Одобрено'))
  })

  it('keeps the form and shows the reason when the service refuses it', async () => {
    const user = userEvent.setup()
    requests.rejectSubmit = 'invalid request fields: justification is too short'
    threadWith()
    renderApp('/')
    const f = await form()
    await user.click(await within(f).findByRole('button', { name: 'Отправить заявку' }))
    expect(await within(f).findByRole('alert')).toHaveTextContent(
      'Сервис заявок не принял: invalid request fields: justification is too short',
    )
    expect(within(f).getByRole('button', { name: 'Отправить заявку' })).toBeEnabled()
  })

  it('shows an already sent request instead of the form after a reload', async () => {
    const t = threadWith()
    requests.requests = [pendingRequest(t.id, 'access.vpn', { justification: 'Работа со стендами из дома' })]
    renderApp('/')
    const card = await screen.findByRole('region', { name: 'Заявка REQ-1: Доступ к корпоративному VPN' })
    expect(card).toHaveTextContent('Ждёт согласования')
    expect(screen.queryByRole('form', { name: /VPN/ })).not.toBeInTheDocument()
  })

  it('falls back to the prepared values when the catalog is down', async () => {
    const user = userEvent.setup()
    requests.failTypes = true
    threadWith()
    renderApp('/')
    const f = await form()
    expect(await within(f).findByRole('alert')).toHaveTextContent('Не удалось загрузить данные')
    requests.failTypes = false
    await user.click(within(f).getByRole('button', { name: 'Повторить' }))
    expect(await within(f).findByLabelText(/Обоснование/)).toHaveValue('Работа со стендами из дома')
  })
})

describe('request status events', () => {
  it('reloads the requests when the status is for one it does not know yet', async () => {
    const t = threadWith()
    renderApp('/')
    await form()
    requests.requests = [pendingRequest(t.id, 'access.vpn', { justification: 'Работа со стендами из дома' })]
    emit(t.id, { event: 'request_status', data: { request_id: 'REQ-1', status: 'pending' } })
    expect(await screen.findByRole('region', { name: /Заявка REQ-1/ })).toHaveTextContent('Ждёт согласования')
  })
})
