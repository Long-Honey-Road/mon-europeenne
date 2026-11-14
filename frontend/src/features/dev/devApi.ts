import { useSession } from '../auth/session'

// A bare client for the service page only: the product screens use the typed client in src/api.
const base = () => import.meta.env.VITE_API_URL ?? ''

export async function devCall<T = Record<string, unknown>>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`${base()}${path}`, {
    method,
    headers: {
      Authorization: `Bearer ${useSession.getState().token ?? ''}`,
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const data = (await res.json().catch(() => ({}))) as Record<string, unknown>
  if (!res.ok) throw new Error(typeof data.message === 'string' ? data.message : `${res.status}`)
  return data as T
}

export type Focus = { kind: 'FOCUS_KIND_PLAN_ITEM' | 'FOCUS_KIND_VAULT_ITEM'; id: string } | null
export type DevMessage = { id: string; role: string; text: string; focus?: { kind: string; id: string } }
export type DevPin = { id: string; title: string; text: string }
export type DevVaultItem = {
  id: string
  title: string
  summary: string
  tags: string[]
  version: number
  versions?: DevVaultItem[]
}
