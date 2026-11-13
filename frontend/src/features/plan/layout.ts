import type { PlanItem, PlanPeriod } from '../../api/schemas'
import { periodTitle } from '../today/useToday'

export const periodOrder: PlanPeriod[] = [
  'PLAN_PERIOD_W1',
  'PLAN_PERIOD_W2',
  'PLAN_PERIOD_M1',
  'PLAN_PERIOD_M2',
  'PLAN_PERIOD_EQUATOR',
  'PLAN_PERIOD_FINAL',
  'PLAN_PERIOD_UNSPECIFIED',
]

const ROW = 190
const PAD = 50
const TOP = 24

export type MapNode = { item: PlanItem; x: number; y: number }
export type MapBand = { period: PlanPeriod; title: string; top: number; height: number }

/**
 * Lays the plan out as constellations: one band per stage, top to bottom; items of a stage spread across in rows of
 * at most `perRow` with a zig-zag so links and labels do not overlap. `x` is in % of the width, `y` in px.
 */
export function layoutPlan(items: PlanItem[], perRow = 8): { nodes: MapNode[]; bands: MapBand[]; height: number } {
  const nodes: MapNode[] = []
  const bands: MapBand[] = []
  let top = TOP
  for (const period of periodOrder) {
    const inBand = items
      .filter((i) => i.period === period)
      .sort((a, b) => (a.map?.y ?? 0) - (b.map?.y ?? 0))
    if (inBand.length === 0) continue
    const rows = Math.ceil(inBand.length / perRow)
    const height = PAD + rows * ROW
    bands.push({ period, title: periodTitle[period] || 'Без этапа', top, height })
    inBand.forEach((item, i) => {
      const row = Math.floor(i / perRow)
      const inRow = Math.min(perRow, inBand.length - row * perRow)
      const k = i % perRow
      const zig = inRow > 1 ? (k % 2 === 0 ? -1 : 1) * (inRow > 4 ? 40 : 26) : 0
      const drift = ((i * 37) % 3) - 1
      nodes.push({
        item,
        x: 8 + ((k + 0.5) / inRow) * 84,
        y: top + PAD + row * ROW + ROW / 2 - 20 + zig + drift * 8,
      })
    })
    top += height
  }
  return { nodes, bands, height: top + TOP }
}
