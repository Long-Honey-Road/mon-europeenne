import { create } from 'zustand'

type PlanHighlight = {
  ids: string[]
  note: string
  set: (ids: string[], note: string) => void
}

/** Plan items buddy pointed at with a highlight event; the map lights them up. */
export const usePlanHighlight = create<PlanHighlight>()((set) => ({
  ids: [],
  note: '',
  set: (ids, note) => set({ ids, note }),
}))

/** Reads a highlight event: `{kind, ids, note}` from §6.5 or the older `{item_ids, note}`. */
export function applyHighlight(data: Record<string, unknown>) {
  if (typeof data.kind === 'string' && data.kind !== 'plan_item') return
  const raw = Array.isArray(data.ids) ? data.ids : Array.isArray(data.item_ids) ? data.item_ids : []
  usePlanHighlight.getState().set(raw.map(String), typeof data.note === 'string' ? data.note : '')
}
