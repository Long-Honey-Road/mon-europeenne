// Synthetic demo data for the «Задачи» orb until S-08. No real Avito employees or regulations.
export type ItemStatus = 'done' | 'active' | 'waiting' | 'todo'

export type InfoItem = { title: string; note: string; status?: ItemStatus }

export const workItems: InfoItem[] = [
  { title: 'MSG-142 · Учебная задача', note: 'Рабочая задача, разбор до кода', status: 'active' },
  { title: 'Доступ к Jira-проекту MSG', note: 'Согласует владелец проекта', status: 'waiting' },
  { title: 'Доступ к VPN', note: 'Выдан сегодня в 11:20', status: 'done' },
]
