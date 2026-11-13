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

const planItem = (
  id: string,
  title: string,
  period: PlanItem['period'],
  status: PlanItem['status'],
  extra: Partial<PlanItem> = {},
): PlanItem => ({ ...item(id, title, status), period, ...extra })

export const plan: { items: PlanItem[]; done: number; total: number } = {
  items: [
    planItem('w1-team', 'Познакомиться с командой', 'PLAN_PERIOD_W1', 'PLAN_ITEM_STATUS_DONE'),
    planItem('w1-access', 'Запросить доступы', 'PLAN_PERIOD_W1', 'PLAN_ITEM_STATUS_DOING', {
      practice: 'Отправь заявки на Jira, Bitbucket и VPN через buddy',
      note: 'VPN ещё на согласовании',
    }),
    planItem('w1-cli', 'Настроить avito-cli', 'PLAN_PERIOD_W1', 'PLAN_ITEM_STATUS_BLOCKED', {
      blocked_by: ['w1-access'],
    }),
    planItem('w2-review', 'Пройти первое ревью', 'PLAN_PERIOD_W2', 'PLAN_ITEM_STATUS_TODO'),
  ],
  done: 1,
  total: 4,
}

type State = {
  today: Today | object
  people: { people: Person[] } | object
  plan: typeof plan | object
  patched: { id: string; status: string }[]
  fail: { today: boolean; people: boolean; plan: boolean; patch: boolean }
}

const fresh = (): State => ({
  today,
  people: { people },
  plan: structuredClone(plan),
  patched: [],
  fail: { today: false, people: false, plan: false, patch: false },
})

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
  http.get(`${API}/v1/private/plan`, () => (panels.fail.plan ? unavailable() : HttpResponse.json(panels.plan))),
  http.patch(`${API}/v1/private/plan/items/:id`, async ({ params, request }) => {
    if (panels.fail.patch) return unavailable()
    const id = String(params.id)
    const body = (await request.json()) as { status: PlanItem['status'] }
    panels.patched.push({ id, status: body.status })
    const state = panels.plan as typeof plan
    const found = state.items.find((i) => i.id === id)
    if (!found) return HttpResponse.json({ code: 5, message: 'not found', details: [] }, { status: 404 })
    found.status = body.status
    state.done = state.items.filter((i) => i.status === 'PLAN_ITEM_STATUS_DONE').length
    return HttpResponse.json({ item: found })
  }),
]
