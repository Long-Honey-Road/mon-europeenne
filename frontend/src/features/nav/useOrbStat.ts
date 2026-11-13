import { plural } from '../../shared/plural'
import { usePeople } from '../team/usePeople'
import { useToday } from '../today/useToday'
import type { AppSection } from './sections'

/** At-a-glance figure on each orb: live for «Сегодня» and «Команда», demo for the rest until S-06. */
export function useOrbStats(): Record<AppSection['id'], string> {
  const today = useToday()
  const people = usePeople()
  const steps = today.data?.next_steps.length
  const team = people.data?.filter((p) => !p.other_team).length
  return {
    today: steps === undefined ? '' : steps === 0 ? 'всё сделано' : `${steps} ${plural(steps, ['шаг', 'шага', 'шагов'])}`,
    plan: '7 из 15',
    work: '1 ждёт',
    team: team === undefined ? '' : `${team} ${plural(team, ['человек', 'человека', 'человек'])}`,
  }
}
