// Synthetic demo data. No real Avito employees or regulations.
export type ItemStatus = 'done' | 'active' | 'waiting' | 'todo'

export type InfoItem = { title: string; note: string; status?: ItemStatus }

export const intern = {
  name: 'Аня',
  role: 'Backend-стажёр',
  team: 'Юнит Messenger',
  day: 3,
  totalDays: 14,
}

export const todayItems: InfoItem[] = [
  { title: 'Запросить доступ к репозиторию команды', note: 'Заявка у руководителя', status: 'waiting' },
  { title: 'Поднять локальное окружение', note: 'По инструкции команды, около часа', status: 'active' },
  { title: 'Прочитать гайд по код-ревью', note: '15 минут перед первой задачей', status: 'todo' },
]

export const planItems: InfoItem[] = [
  { title: 'Знакомство с командой', note: '4 из 4 шагов', status: 'done' },
  { title: 'Рабочие инструменты', note: '3 из 5 шагов', status: 'active' },
  { title: 'Первые задачи', note: '0 из 4 шагов', status: 'todo' },
  { title: 'Обратная связь', note: '0 из 2 шагов', status: 'todo' },
]

export const workItems: InfoItem[] = [
  { title: 'MSG-142 · Учебная задача', note: 'Рабочая задача, разбор до кода', status: 'active' },
  { title: 'Доступ к Jira-проекту MSG', note: 'Согласует владелец проекта', status: 'waiting' },
  { title: 'Доступ к VPN', note: 'Выдан сегодня в 11:20', status: 'done' },
]

export const teamItems: InfoItem[] = [
  { title: 'Марк · наставник', note: 'Задачи, код, ревью' },
  { title: 'Игорь · руководитель', note: 'Цели, отсутствия, доступы' },
  { title: 'Лена · HR BP', note: 'Адаптация, вопросы вне команды' },
  { title: 'Тимур · инфраструктура', note: 'Окружения, пайплайны' },
]
