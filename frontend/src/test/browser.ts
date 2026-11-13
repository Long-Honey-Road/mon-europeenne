import { setupWorker } from 'msw/browser'
import type { PlanItem } from '../api/schemas'
import { handlers } from './api'
import { panels } from './panels'

type SeedItem = { key: string; title: string; period: string; practice?: string; depends_on?: string[]; map?: PlanItem['map'] }
type SeedStatus = { user_id: string; item_key: string; status: string; note?: string }
type SeedPlan = { items: SeedItem[]; statuses: SeedStatus[] }

// The seed lives outside frontend/: a glob finds nothing in a frontend-only build context, so this never breaks it.
const seedFiles = import.meta.glob<SeedPlan>('../../../backend/seed/plan.json', { eager: true, import: 'default' })

// The map looks real only with the full plan: take the synthetic seed the backend loads, with the intern's progress.
function planFromSeed(data: SeedPlan): typeof panels.plan {
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

const seed = Object.values(seedFiles)[0]
if (seed) panels.plan = planFromSeed(seed)

/** The same fake API as in tests, served by a service worker for `npm run dev:mock`. */
export const worker = setupWorker(...handlers)
