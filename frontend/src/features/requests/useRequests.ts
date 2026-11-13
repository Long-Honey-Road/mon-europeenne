import { useQuery } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { api } from '../../api'
import type { RequestStatus, ServiceRequest } from '../../api/schemas'
import { approverSchema } from '../../api/schemas'
import { useSession } from '../auth/session'
import { todayQueryKey } from '../today/useToday'

export const requestTypesKey = ['request-types'] as const
export const myRequestsKey = ['requests'] as const

export function useRequestTypes() {
  const signedIn = useSession((s) => s.token !== null)
  return useQuery({ queryKey: requestTypesKey, queryFn: api.requestTypes, enabled: signedIn, staleTime: Infinity })
}

export function useMyRequests() {
  const signedIn = useSession((s) => s.token !== null)
  return useQuery({ queryKey: myRequestsKey, queryFn: api.myRequests, enabled: signedIn })
}

/** Puts a just-sent request into the cache, so the thread shows its status without a refetch. */
export function rememberRequest(qc: QueryClient, r: ServiceRequest) {
  qc.setQueryData<ServiceRequest[]>(myRequestsKey, (list) => [r, ...(list ?? []).filter((x) => x.id !== r.id)])
  void qc.invalidateQueries({ queryKey: todayQueryKey })
}

const statusFromEvent: Record<string, RequestStatus> = {
  pending: 'REQUEST_STATUS_PENDING',
  approved: 'REQUEST_STATUS_APPROVED',
  rejected: 'REQUEST_STATUS_REJECTED',
}

/** Applies a request_status event from the thread stream to the cached requests and the today blockers. */
export function applyRequestStatus(qc: QueryClient, data: Record<string, unknown>) {
  const id = typeof data.request_id === 'string' ? data.request_id : ''
  const status = statusFromEvent[String(data.status)]
  const known = qc.getQueryData<ServiceRequest[]>(myRequestsKey)?.some((r) => r.id === id)
  if (id && status && known) {
    const approver = approverSchema.safeParse(data.approver)
    qc.setQueryData<ServiceRequest[]>(myRequestsKey, (list) =>
      (list ?? []).map((r) =>
        r.id === id
          ? {
              ...r,
              status,
              approver: approver.success ? approver.data : r.approver,
              next_step: typeof data.next_step === 'string' ? data.next_step : r.next_step,
              reason: typeof data.reason === 'string' ? data.reason : r.reason,
            }
          : r,
      ),
    )
  } else {
    void qc.invalidateQueries({ queryKey: myRequestsKey })
  }
  void qc.invalidateQueries({ queryKey: todayQueryKey })
}
