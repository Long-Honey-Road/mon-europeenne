import { useQuery } from '@tanstack/react-query'
import { api } from '../../api'
import type { Role } from '../../api/schemas'
import { useSession } from './session'

export const meQueryKey = ['me'] as const

/** Profile of the signed-in user; disabled without a session. */
export function useMe() {
  const token = useSession((s) => s.token)
  return useQuery({ queryKey: meQueryKey, queryFn: api.me, enabled: token !== null })
}

const roleTitle: Record<Role, string> = {
  ROLE_UNSPECIFIED: '',
  ROLE_INTERN: 'стажёр',
  ROLE_MENTOR: 'наставник',
  ROLE_MANAGER: 'руководитель',
  ROLE_HR_BP: 'HR BP',
  ROLE_HR_ADMIN: 'HR admin',
}

export type CurrentUser = { name: string; firstName: string; subtitle: string }

/** Who is signed in, from the profile or, while it loads or fails, from the sign-in response. */
export function useCurrentUser(): CurrentUser | undefined {
  const me = useMe()
  const saved = useSession((s) => s.user)
  const user = me.data?.user ?? saved
  if (!user) return undefined
  const team = me.data?.team?.name
  return {
    name: user.display_name,
    firstName: user.display_name.split(/\s+/)[0] ?? '',
    subtitle: [roleTitle[user.role], team].filter(Boolean).join(' · '),
  }
}
