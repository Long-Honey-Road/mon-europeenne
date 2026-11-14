import { describe, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import type { User } from '../../api/schemas'
import { API, intern, me } from '../../test/api'
import { renderApp } from '../../test/render'
import { server } from '../../test/server'
import { loginFromName } from './hire'

const manager: User = { ...intern, id: 'u-dmitry', login: 'manager', display_name: 'Дмитрий Козлов', role: 'ROLE_MANAGER' }

type Sent = { display_name: string; login: string; direction: string; mentor_id: string; start_date: string }

/** Signs in as the manager and fakes the hiring endpoint; returns what it received. */
function asManager(reply?: (body: Sent) => Response) {
  const sent: Sent[] = []
  server.use(
    http.get(`${API}/v1/private/me`, () => HttpResponse.json({ ...me, user: manager, relations: null })),
    http.post(`${API}/v1/private/team/interns`, async ({ request }) => {
      const body = (await request.json()) as Sent
      sent.push(body)
      if (reply) return reply(body)
      return HttpResponse.json({
        user: { ...intern, id: `u-${body.login}`, login: body.login, display_name: body.display_name },
        login: body.login,
        temporary_password: 'Kq7mPx2RtW9z',
      })
    }),
  )
  return sent
}

const panel = () => screen.findByRole('region', { name: 'Команда' })

async function openForm() {
  const user = userEvent.setup()
  renderApp('/team')
  const p = await panel()
  await user.click(await within(p).findByRole('button', { name: 'Добавить стажёра' }))
  return { user, form: within(p).getByRole('form', { name: 'Новый стажёр' }), p }
}

describe('a manager adds an intern', () => {
  it('fills the login from the name, sends the form and shows the one-time password once', async () => {
    const sent = asManager()
    const { user, form, p } = await openForm()
    await user.type(within(form).getByLabelText(/Имя и фамилия/), 'Вера Новикова')
    expect(within(form).getByLabelText(/Логин/)).toHaveValue('vnovikova')
    await user.selectOptions(within(form).getByLabelText(/Направление/), 'DIRECTION_QA')
    await user.selectOptions(within(form).getByLabelText(/Наставник/), 'p-anna')
    await user.clear(within(form).getByLabelText(/Дата выхода/))
    await user.type(within(form).getByLabelText(/Дата выхода/), '2026-11-16')
    await user.click(within(form).getByRole('button', { name: 'Добавить' }))

    const done = await within(p).findByRole('region', { name: 'Стажёр добавлен' })
    expect(done).toHaveTextContent('Вера Новикова')
    expect(done).toHaveTextContent('vnovikova')
    expect(done).toHaveTextContent('Kq7mPx2RtW9z')
    expect(done).toHaveTextContent('показывается один раз')
    expect(sent).toEqual([
      { display_name: 'Вера Новикова', login: 'vnovikova', direction: 'DIRECTION_QA', mentor_id: 'p-anna', start_date: '2026-11-16' },
    ])

    await user.click(within(done).getByRole('button', { name: 'Готово' }))
    expect(within(p).queryByText('Kq7mPx2RtW9z')).not.toBeInTheDocument()
    expect(within(p).getByRole('button', { name: 'Добавить стажёра' })).toBeInTheDocument()
  })

  it('keeps a login the manager typed by hand', async () => {
    asManager()
    const { user, form } = await openForm()
    await user.type(within(form).getByLabelText(/Логин/), 'vera.n')
    await user.type(within(form).getByLabelText(/Имя и фамилия/), 'Вера Новикова')
    expect(within(form).getByLabelText(/Логин/)).toHaveValue('vera.n')
  })

  it('offers only own team members who are not interns as mentors', async () => {
    asManager()
    const { form } = await openForm()
    const options = within(within(form).getByLabelText(/Наставник/)).getAllByRole('option').map((o) => o.textContent)
    expect(options).toEqual(['Выбери наставника', 'Анна Белова', 'Игорь Волков'])
  })

  it('checks the fields before sending', async () => {
    const sent = asManager()
    const { user, form } = await openForm()
    await user.click(within(form).getByRole('button', { name: 'Добавить' }))
    expect(within(form).getByRole('alert')).toHaveTextContent('Укажи имя, логин и наставника')
    await user.type(within(form).getByLabelText(/Имя и фамилия/), 'Вера Новикова')
    await user.clear(within(form).getByLabelText(/Логин/))
    await user.type(within(form).getByLabelText(/Логин/), 'Вера')
    await user.selectOptions(within(form).getByLabelText(/Наставник/), 'p-anna')
    await user.click(within(form).getByRole('button', { name: 'Добавить' }))
    expect(within(form).getByRole('alert')).toHaveTextContent('Логин: латиница в нижнем регистре')
    expect(sent).toHaveLength(0)
  })

  it('says when the login is taken and keeps the form', async () => {
    asManager(() => HttpResponse.json({ code: 6, message: 'login "vnovikova" is taken', details: [] }, { status: 409 }))
    const { user, form } = await openForm()
    await user.type(within(form).getByLabelText(/Имя и фамилия/), 'Вера Новикова')
    await user.selectOptions(within(form).getByLabelText(/Наставник/), 'p-anna')
    await user.click(within(form).getByRole('button', { name: 'Добавить' }))
    expect(await within(form).findByRole('alert')).toHaveTextContent('Логин vnovikova уже занят')
    expect(within(form).getByLabelText(/Имя и фамилия/)).toHaveValue('Вера Новикова')
  })

  it.each([
    [400, 3, 'mentor must be a person of the manager'],
    [403, 7, 'Добавлять стажёров может только руководитель'],
    [501, 12, 'Сервис учёток не подключён'],
    [503, 14, 'Сервис учёток не отвечает'],
  ])('explains a %i refusal', async (status, code, text) => {
    asManager(() =>
      HttpResponse.json({ code, message: "mentor must be a person of the manager's team", details: [] }, { status }),
    )
    const { user, form } = await openForm()
    await user.type(within(form).getByLabelText(/Имя и фамилия/), 'Вера Новикова')
    await user.selectOptions(within(form).getByLabelText(/Наставник/), 'p-anna')
    await user.click(within(form).getByRole('button', { name: 'Добавить' }))
    expect(await within(form).findByRole('alert')).toHaveTextContent(text)
  })

  it('is not offered to an intern', async () => {
    renderApp('/team')
    const p = await panel()
    await within(p).findByText('Анна Белова')
    await waitFor(() => expect(within(p).queryByRole('button', { name: 'Добавить стажёра' })).not.toBeInTheDocument())
  })
})

describe('login from a Russian name', () => {
  it.each([
    ['Вера Новикова', 'vnovikova'],
    ['  Юлия   Щукина ', 'yshchukina'],
    ['Пётр Ёлкин-Жуков', 'pelkin-zhukov'],
    ['Анна', 'anna'],
    ['John Smith', 'jsmith'],
    ['', ''],
  ])('%s → %s', (name, login) => expect(loginFromName(name)).toBe(login))
})
