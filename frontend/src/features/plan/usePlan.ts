import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api'
import type { Plan, PlanItemStatus } from '../../api/schemas'
import { useSession } from '../auth/session'
import { todayQueryKey } from '../today/useToday'

export const planQueryKey = ['plan'] as const

export function usePlan() {
  const signedIn = useSession((s) => s.token !== null)
  return useQuery({ queryKey: planQueryKey, queryFn: api.plan, enabled: signedIn })
}

/** Saves an item status, then puts the saved item into the plan and refreshes «Сегодня». */
export function useSetItemStatus() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, status }: { id: string; status: PlanItemStatus }) => api.updatePlanItem(id, status),
    onSuccess: (item) => {
      qc.setQueryData<Plan>(planQueryKey, (plan) => {
        if (!plan) return plan
        const items = plan.items.map((i) => (i.id === item.id ? { ...i, ...item } : i))
        return { ...plan, items, done: items.filter((i) => i.status === 'PLAN_ITEM_STATUS_DONE').length }
      })
      void qc.invalidateQueries({ queryKey: todayQueryKey })
      // Dependencies may have unblocked other items.
      void qc.invalidateQueries({ queryKey: planQueryKey })
    },
  })
}
