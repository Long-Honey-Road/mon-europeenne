import { useQuery } from '@tanstack/react-query'
import { api } from '../../api'
import type { PlanItemStatus, PlanPeriod } from '../../api/schemas'
import { useSession } from '../auth/session'

export const todayQueryKey = ['today'] as const

export function useToday() {
  const signedIn = useSession((s) => s.token !== null)
  return useQuery({ queryKey: todayQueryKey, queryFn: api.today, enabled: signedIn })
}

export const periodTitle: Record<PlanPeriod, string> = {
  PLAN_PERIOD_UNSPECIFIED: '',
  PLAN_PERIOD_W1: '1 неделя',
  PLAN_PERIOD_W2: '2 неделя',
  PLAN_PERIOD_M1: '1 месяц',
  PLAN_PERIOD_M2: '2 месяц',
  PLAN_PERIOD_EQUATOR: 'экватор',
  PLAN_PERIOD_FINAL: 'выпуск',
}

export type StatusTone = 'done' | 'active' | 'waiting' | 'todo'

export const itemStatus: Record<PlanItemStatus, { tone: StatusTone; text: string }> = {
  PLAN_ITEM_STATUS_UNSPECIFIED: { tone: 'todo', text: 'Впереди' },
  PLAN_ITEM_STATUS_TODO: { tone: 'todo', text: 'Впереди' },
  PLAN_ITEM_STATUS_DOING: { tone: 'active', text: 'В работе' },
  PLAN_ITEM_STATUS_BLOCKED: { tone: 'waiting', text: 'Заблокирован' },
  PLAN_ITEM_STATUS_DONE: { tone: 'done', text: 'Готово' },
}

/** «День 3 · неделя 1», or a note before the start date. */
export function dayLabel(day: number, week: number): string {
  return day > 0 ? `День ${day} · неделя ${week}` : 'Стажировка ещё не началась'
}
