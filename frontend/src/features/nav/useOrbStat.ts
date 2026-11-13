import { plural } from '../../shared/plural'
import { usePlan } from '../plan/usePlan'
import { useMyRequests } from '../requests/useRequests'
import { taskOf, useThreads } from '../task/useTasks'
import { usePeople } from '../team/usePeople'
import { useToday } from '../today/useToday'
import type { AppSection } from './sections'

/** At-a-glance figure on each orb, from the same queries as the panels. */
export function useOrbStats(): Record<AppSection['id'], string> {
  const today = useToday()
  const people = usePeople()
  const plan = usePlan().data
  const threads = useThreads().data
  const requests = useMyRequests().data
  const steps = today.data?.next_steps.length
  const team = people.data?.filter((p) => !p.other_team).length
  return {
    today: steps === undefined ? '' : steps === 0 ? 'всё сделано' : `${steps} ${plural(steps, ['шаг', 'шага', 'шагов'])}`,
    plan: plan ? `${plan.done} из ${plan.total}` : '',
    work: workStat(threads?.filter((t) => taskOf(t)).length, requests?.filter((r) => r.status === 'REQUEST_STATUS_PENDING').length),
    team: team === undefined ? '' : `${team} ${plural(team, ['человек', 'человека', 'человек'])}`,
  }
}

function workStat(tasks: number | undefined, waiting: number | undefined): string {
  if (tasks === undefined || waiting === undefined) return ''
  const parts = [tasks > 0 ? `${tasks} ${plural(tasks, ['задача', 'задачи', 'задач'])}` : 'нет задач']
  if (waiting > 0) parts.push(`${waiting} ждёт`)
  return parts.join(' · ')
}
