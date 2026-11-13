import { setupWorker } from 'msw/browser'
import type { PlanItem } from '../api/schemas'
import seedPlan from '../../../backend/seed/plan.json'
import { handlers } from './api'
import { panels } from './panels'

type SeedItem = { key: string; title: string; period: string; practice?: string; depends_on?: string[]; map?: PlanItem['map'] }
type SeedStatus = { user_id: string; item_key: string; status: string; note?: string }

// The map looks real only with the full plan: take the synthetic seed the backend loads, with the intern's progress.
function planFromSeed(): typeof panels.plan {
  const data = seedPlan as { items: SeedItem[]; statuses: SeedStatus[] }
  const progress = new Map(data.statuses.filter((s) => s.user_id === 'u-liza').map((s) => [s.item_key, s]))
  const status = (key: string) =>
    `PLAN_ITEM_STATUS_${(progress.get(key)?.status ?? 'todo').toUpperCase()}` as PlanItem['status']
  const items: PlanItem[] = data.items.map((i) => ({
    id: i.key,
    title: i.title,
    practice: i.practice ?? '',
    period: `PLAN_PERIOD_${i.period.toUpperCase()}` as PlanItem['period'],
    status: status(i.key),
    note: progress.get(i.key)?.note ?? '',
    depends_on: i.depends_on ?? [],
    blocked_by: (i.depends_on ?? []).filter((d) => status(d) !== 'PLAN_ITEM_STATUS_DONE'),
    map: i.map,
  }))
  return { items, done: items.filter((i) => i.status === 'PLAN_ITEM_STATUS_DONE').length, total: items.length }
}

panels.plan = planFromSeed()

/** The same fake API as in tests, served by a service worker for `npm run dev:mock`. */
export const worker = setupWorker(...handlers)
