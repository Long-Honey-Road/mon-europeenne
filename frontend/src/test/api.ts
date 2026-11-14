import { HttpResponse, http } from 'msw'
import type { LoginResponse, Me, User } from '../api/schemas'
import { chatHandlers } from './chat'
import { addPerson, panelHandlers } from './panels'
import { requestHandlers } from './requests'

// Mirrors the backend contract in buddy-api.yaml with synthetic data.
export const API = 'http://api.test'
export const TOKEN = 'test-token'

export const intern: User = {
  id: 'p-intern',
  login: 'intern',
  display_name: 'Аня Соколова',
  role: 'ROLE_INTERN',
  direction: 'DIRECTION_BACKEND',
  team_id: 't-messenger',
  repos: ['messenger-api'],
  jira_projects: ['MSG'],
  location: 'Москва',
  start_date: '2026-11-09',
}

export const MANAGER_TOKEN = 'test-token-manager'

export const manager: User = {
  ...intern,
  id: 'u-dmitry',
  login: 'manager',
  display_name: 'Дмитрий Козлов',
  role: 'ROLE_MANAGER',
}

export const me: Me = {
  user: intern,
  team: { id: 't-messenger', name: 'Messenger', unit: 'Коммуникации', cluster: 'C2C', mattermost_channel: '~messenger' },
  relations: {
    mentor: { id: 'p-mentor', name: 'Марк Ли', role_title: 'Senior backend' },
    manager: { id: 'p-manager', name: 'Игорь Петров', role_title: 'Team lead' },
    hr_bp: undefined,
  },
}

const unauthenticated = (message: string) => HttpResponse.json({ code: 16, message, details: [] }, { status: 401 })

export const handlers = [
  http.post(`${API}/v1/public/auth/login`, async ({ request }) => {
    const body = (await request.json()) as { login?: string; password?: string }
    if (body.login === 'intern' && body.password === 'intern-demo') {
      return HttpResponse.json({ token: TOKEN, user: intern } satisfies LoginResponse)
    }
    if (body.login === 'manager' && body.password === 'manager-demo') {
      return HttpResponse.json({ token: MANAGER_TOKEN, user: manager } satisfies LoginResponse)
    }
    return unauthenticated('invalid login or password')
  }),
  http.get(`${API}/v1/private/me`, ({ request }) => {
    if (request.headers.get('Authorization') === `Bearer ${MANAGER_TOKEN}`) {
      return HttpResponse.json({ ...me, user: manager, relations: null })
    }
    if (request.headers.get('Authorization') !== `Bearer ${TOKEN}`) return unauthenticated('authentication required')
    // protojson sends unset messages as null.
    return HttpResponse.json({ ...me, relations: { ...me.relations, hr_bp: null } })
  }),
  // A manager hires an intern: the person shows up in the team, the password is a fixed test one.
  http.post(`${API}/v1/private/team/interns`, async ({ request }) => {
    const body = (await request.json()) as { display_name: string; login: string; mentor_id: string }
    addPerson({ id: `p-${body.login}`, name: body.display_name, role_title: 'Стажёр' })
    return HttpResponse.json({
      user: { ...intern, id: `u-${body.login}`, login: body.login, display_name: body.display_name },
      login: body.login,
      temporary_password: 'Kq7mPx2RtW9z',
    })
  }),
  ...chatHandlers,
  ...panelHandlers,
  ...requestHandlers,
]

