import { HttpResponse, http } from 'msw'
import type { Person, PlanItem, Today } from '../api/schemas'

// Fake /today and /people with synthetic data in the shape the backend sends.
const API = 'http://api.test'

const item = (id: string, title: string, status: PlanItem['status'], practice = ''): PlanItem => ({
  id,
  title,
  practice,
  period: 'PLAN_PERIOD_W1',
  status,
  note: '',
  blocked_by: [],
})

export const today: Today = {
  day: 3,
  week: 1,
  period: 'PLAN_PERIOD_W1',
  next_steps: [
    item('w1-env', 'Поднять локальное окружение', 'PLAN_ITEM_STATUS_DOING', 'Запусти сервис по README команды'),
    item('w1-review', 'Прочитать гайд по код-ревью', 'PLAN_ITEM_STATUS_TODO'),
  ],
  blockers: [
    { kind: 'request', id: 'r-1', title: 'Доступ к репозиторию messenger-api', reason: 'Ждёт согласования руководителя' },
    { kind: 'access_missing', id: 'jira', title: 'Доступ к Jira', reason: 'Заявка ещё не отправлена' },
  ],
  done: 4,
  total: 17,
}

const person = (p: Partial<Person> & Pick<Person, 'id' | 'name' | 'role_title'>): Person => ({
  team_id: 't-messenger',
  responsibilities: [],
  ask_me_about: [],
  contact: '',
  other_team: false,
  ...p,
})

export const people: Person[] = [
  person({
    id: 'p-anna',
    name: 'Анна Белова',
    role_title: 'Senior backend, наставник',
    responsibilities: ['Код-ревью стажёров'],
    ask_me_about: ['архитектура сервиса', 'первая задача'],
    contact: '@anna.belova',
  }),
  person({ id: 'p-igor', name: 'Игорь Волков', role_title: 'Инфраструктура юнита', ask_me_about: ['деплой'] }),
  person({
    id: 'p-artem',
    name: 'Артём Фёдоров',
    role_title: 'Владелец PaaS',
    team_id: 't-platform',
    other_team: true,
  }),
]

type State = { today: Today | object; people: { people: Person[] } | object; fail: { today: boolean; people: boolean } }

const fresh = (): State => ({ today, people: { people }, fail: { today: false, people: false } })

/** Mutable state of the fake; tests change it, `resetPanels` restores the defaults. */
export let panels = fresh()

export function resetPanels() {
  panels = fresh()
}

const unavailable = () =>
  HttpResponse.json({ code: 14, message: 'service unavailable', details: [] }, { status: 503 })

export const panelHandlers = [
  http.get(`${API}/v1/private/today`, () => (panels.fail.today ? unavailable() : HttpResponse.json(panels.today))),
  http.get(`${API}/v1/private/people`, () => (panels.fail.people ? unavailable() : HttpResponse.json(panels.people))),
]
