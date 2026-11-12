import { useSession } from '../features/auth/session'
import { createApi } from './client'

export { ApiError } from './client'
export type { Api } from './client'

export const api = createApi({
  baseUrl: import.meta.env.VITE_API_URL ?? '',
  getToken: () => useSession.getState().token,
  onUnauthorized: () => useSession.getState().signOut(),
})
