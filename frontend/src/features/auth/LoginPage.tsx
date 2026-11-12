import { useState } from 'react'
import type { FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { api, ApiError } from '../../api'
import { sections } from '../nav/sections'
import { useSession } from './session'
import './login.css'

// Synthetic accounts from backend/seed/people.json, also listed in the README.
export const testAccounts = [
  { label: 'Стажёр', login: 'intern', password: 'intern-demo' },
  { label: 'Наставник', login: 'mentor', password: 'mentor-demo' },
  { label: 'Руководитель', login: 'manager', password: 'manager-demo' },
] as const

function errorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.kind === 'unauthorized') return 'Неверный логин или пароль.'
    // 502–504 come from the dev proxy or nginx when the API is down.
    if (error.kind === 'network' || [502, 503, 504].includes(error.status)) return 'Сервер не отвечает. Проверьте, что API запущен, и попробуйте ещё раз.'
  }
  return 'Не получилось войти. Попробуйте ещё раз.'
}

export function LoginPage() {
  const navigate = useNavigate()
  const { redirect } = useSearch({ strict: false }) as { redirect?: string }
  const signIn = useSession((s) => s.signIn)
  const queryClient = useQueryClient()
  const [login, setLogin] = useState('')
  const [password, setPassword] = useState('')

  const mutation = useMutation({
    mutationFn: (creds: { login: string; password: string }) => api.login(creds.login, creds.password),
    onSuccess: ({ token, user }) => {
      queryClient.clear()
      signIn(token, user)
      // Only same-app paths, never an absolute URL from the query string.
      const to = redirect?.startsWith('/') && !redirect.startsWith('//') ? redirect : '/'
      void navigate({ to })
    },
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!login.trim() || !password) return
    mutation.mutate({ login: login.trim(), password })
  }

  const pending = mutation.isPending

  return (
    <main className="login">
      <div className="login__card">
        <span className="logo login__logo" aria-hidden="true">
          {sections.map((s) => (
            <i
              key={s.id}
              data-accent={s.accent}
              style={{ left: `${s.orb.x}%`, top: `${s.orb.y}%`, width: `${s.orb.d}%` }}
            />
          ))}
        </span>
        <h1 className="login__title">Вход в AI-buddy</h1>
        <p className="login__lead">Учебный прототип. Учётные записи тестовые, данные синтетические.</p>

        <form className="login__form" onSubmit={submit} noValidate>
          <label className="login__field">
            <span className="label">Логин</span>
            <input
              className="login__input"
              name="login"
              autoComplete="username"
              value={login}
              onChange={(e) => setLogin(e.target.value)}
              disabled={pending}
            />
          </label>
          <label className="login__field">
            <span className="label">Пароль</span>
            <input
              className="login__input"
              name="password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              disabled={pending}
            />
          </label>

          {mutation.isError && (
            <p className="login__error" role="alert">
              {errorText(mutation.error)}
            </p>
          )}

          <button className="login__submit" type="submit" disabled={pending || !login.trim() || !password}>
            {pending ? 'Входим…' : 'Войти'}
          </button>
        </form>

        <div className="login__demo">
          <p className="label">Тестовые учётки</p>
          <div className="login__chips">
            {testAccounts.map((a) => (
              <button
                key={a.login}
                type="button"
                className="login__chip"
                disabled={pending}
                onClick={() => {
                  setLogin(a.login)
                  setPassword(a.password)
                  mutation.reset()
                }}
              >
                {a.label}
              </button>
            ))}
          </div>
        </div>
      </div>
    </main>
  )
}
