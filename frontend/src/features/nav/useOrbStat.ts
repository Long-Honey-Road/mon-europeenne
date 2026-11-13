import { plural } from '../../shared/plural'
import { usePlan } from '../plan/usePlan'
import { usePeople } from '../team/usePeople'
import { useToday } from '../today/useToday'
import type { AppSection } from './sections'

/** At-a-glance figure on each orb: live except «Задачи», which waits for S-08. */
export function useOrbStats(): Record<AppSection['id'], string> {
  const today = useToday()
  const people = usePeople()
  const plan = usePlan().data
  const steps = today.data?.next_steps.length
  const team = people.data?.filter((p) => !p.other_team).length
  return {
    today: steps === undefined ? '' : steps === 0 ? 'всё сделано' : `${steps} ${plural(steps, ['шаг', 'шага', 'шагов'])}`,
    plan: plan ? `${plan.done} из ${plan.total}` : '',
    work: '1 ждёт',
    team: team === undefined ? '' : `${team} ${plural(team, ['человек', 'человека', 'человек'])}`,
  }
}
