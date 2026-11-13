import { useQuery } from '@tanstack/react-query'
import { api } from '../../api'
import { useSession } from '../auth/session'

export const peopleQueryKey = ['people'] as const

export function usePeople() {
  const signedIn = useSession((s) => s.token !== null)
  return useQuery({ queryKey: peopleQueryKey, queryFn: api.people, enabled: signedIn })
}
