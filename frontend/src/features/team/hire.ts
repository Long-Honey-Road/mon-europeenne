import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '../../api'
import type { NewIntern } from '../../api/client'
import { peopleQueryKey } from './usePeople'

export const LOGIN_RE = /^[a-z][a-z0-9._-]{2,31}$/

const ru: Record<string, string> = {
  а: 'a', б: 'b', в: 'v', г: 'g', д: 'd', е: 'e', ё: 'e', ж: 'zh', з: 'z', и: 'i', й: 'y', к: 'k', л: 'l',
  м: 'm', н: 'n', о: 'o', п: 'p', р: 'r', с: 's', т: 't', у: 'u', ф: 'f', х: 'kh', ц: 'ts', ч: 'ch', ш: 'sh',
  щ: 'shch', ъ: '', ы: 'y', ь: '', э: 'e', ю: 'yu', я: 'ya',
}

const latin = (s: string) =>
  [...s.toLowerCase()]
    .map((c) => ru[c] ?? c)
    .join('')
    .replace(/[^a-z0-9._-]/g, '')

/** Company-style login: first letter of the first name and the surname, «Вера Новикова» → vnovikova. */
export function loginFromName(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean).map(latin).filter(Boolean)
  if (words.length === 0) return ''
  const login = words.length === 1 ? words[0]! : `${words[0]![0]}${words[words.length - 1]}`
  return login.slice(0, 32)
}

/** Text for a refused hire; the server's own words only where they say what to fix. */
export function hireError(error: unknown, login: string): string {
  if (error instanceof ApiError) {
    if (error.status === 409) return `Логин ${login} уже занят: выбери другой.`
    if (error.status === 400) return `Сервис учёток не принял данные: ${error.message}`
    if (error.status === 403) return 'Добавлять стажёров может только руководитель своей команды.'
    if (error.status === 501) return 'Сервис учёток не подключён: в этой сборке buddy работает на тестовых учётках.'
  }
  return 'Сервис учёток не отвечает. Попробуй ещё раз чуть позже.'
}

export function useAddIntern() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (intern: NewIntern) => api.addIntern(intern),
    onSuccess: () => qc.invalidateQueries({ queryKey: peopleQueryKey }),
  })
}
