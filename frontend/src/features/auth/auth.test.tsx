import { describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { API, TOKEN, intern, server } from '../../test/api'
import { renderApp } from '../../test/render'
import { useSession } from './session'

const fill = async (login: string, password: string) => {
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('Логин'), login)
  await user.type(screen.getByLabelText('Пароль'), password)
  await user.click(screen.getByRole('button', { name: 'Войти' }))
}

describe('sign-in', () => {
  it('sends a guest to the login page and remembers where they were going', async () => {
    const { router } = renderApp('/plan', { signedIn: false })
    expect(await screen.findByRole('heading', { name: 'Вход в AI-buddy' })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/login')
    expect(router.state.location.search).toEqual({ redirect: '/plan' })
  })

  it('signs in with a test account and returns to the requested section', async () => {
    const { router } = renderApp('/login?redirect=%2Fplan', { signedIn: false })
    await fill('intern', 'intern-demo')

    expect(await screen.findByRole('heading', { name: 'План адаптации' })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/plan')
    expect(useSession.getState()).toMatchObject({ token: TOKEN, user: intern })
  })

  it('ignores a redirect that leaves the app', async () => {
    const { router } = renderApp('/login?redirect=%2F%2Fevil.example', { signedIn: false })
    await fill('intern', 'intern-demo')
    expect(await screen.findByRole('heading', { name: /Привет, Аня/ })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/')
  })

  it('fills the form from a test account chip', async () => {
    const user = userEvent.setup()
    renderApp('/login', { signedIn: false })
    await user.click(await screen.findByRole('button', { name: 'Стажёр' }))
    expect(screen.getByLabelText('Логин')).toHaveValue('intern')
    expect(screen.getByLabelText('Пароль')).toHaveValue('intern-demo')
  })

  it('keeps the button disabled until both fields are filled', async () => {
    const user = userEvent.setup()
    renderApp('/login', { signedIn: false })
    const button = await screen.findByRole('button', { name: 'Войти' })
    expect(button).toBeDisabled()
    await user.type(screen.getByLabelText('Логин'), 'intern')
    expect(button).toBeDisabled()
  })

  it('says the password is wrong and stays on the page', async () => {
    const { router } = renderApp('/login', { signedIn: false })
    await fill('intern', 'wrong')
    expect(await screen.findByRole('alert')).toHaveTextContent('Неверный логин или пароль')
    expect(router.state.location.pathname).toBe('/login')
    expect(useSession.getState().token).toBeNull()
  })

  it('explains that the server is down', async () => {
    server.use(http.post(`${API}/v1/public/auth/login`, () => HttpResponse.error()))
    renderApp('/login', { signedIn: false })
    await fill('intern', 'intern-demo')
    expect(await screen.findByRole('alert')).toHaveTextContent('Сервер не отвечает')
  })

  it('shows a generic message for an unexpected failure', async () => {
    server.use(http.post(`${API}/v1/public/auth/login`, () => HttpResponse.json({}, { status: 500 })))
    renderApp('/login', { signedIn: false })
    await fill('intern', 'intern-demo')
    expect(await screen.findByRole('alert')).toHaveTextContent('Не получилось войти')
  })

  it('locks the form while the request is in flight', async () => {
    let release = () => {}
    server.use(
      http.post(`${API}/v1/public/auth/login`, async () => {
        await new Promise<void>((r) => (release = r))
        return HttpResponse.json({ token: TOKEN, user: intern })
      }),
    )
    renderApp('/login', { signedIn: false })
    await fill('intern', 'intern-demo')
    expect(await screen.findByRole('button', { name: 'Входим…' })).toBeDisabled()
    expect(screen.getByLabelText('Логин')).toBeDisabled()
    release()
    expect(await screen.findByRole('heading', { name: /Привет/ })).toBeInTheDocument()
  })

  it('sends a signed-in user away from the login page', async () => {
    const { router } = renderApp('/login')
    await screen.findByRole('navigation', { name: 'Разделы' })
    expect(router.state.location.pathname).toBe('/')
  })
})

describe('signed-in user', () => {
  it('greets by first name and shows role and team from the profile', async () => {
    renderApp('/')
    expect(await screen.findByRole('heading', { name: 'Привет, Аня' })).toBeInTheDocument()
    expect(await screen.findByText(/стажёр · Messenger/)).toBeInTheDocument()
    expect(screen.getByText('Аня Соколова')).toBeInTheDocument()
  })

  it('signs out back to the login page', async () => {
    const user = userEvent.setup()
    const { router } = renderApp('/')
    await user.click(await screen.findByRole('button', { name: 'Выйти' }))
    expect(await screen.findByRole('heading', { name: 'Вход в AI-buddy' })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/login')
    expect(useSession.getState().token).toBeNull()
  })

  it('leaves the app when the server rejects the session', async () => {
    useSession.getState().signIn('expired', intern)
    const { router } = renderApp('/', { signedIn: false })
    await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
    expect(useSession.getState().token).toBeNull()
  })

  it('keeps the saved name when the profile cannot be loaded', async () => {
    const spy = vi.fn()
    server.use(
      http.get(`${API}/v1/private/me`, () => {
        spy()
        return HttpResponse.json({ code: 14, message: 'directory unavailable', details: [] }, { status: 503 })
      }),
    )
    renderApp('/')
    expect(await screen.findByRole('heading', { name: 'Привет, Аня' })).toBeInTheDocument()
    await waitFor(() => expect(spy).toHaveBeenCalled())
    expect(screen.getByText('Аня Соколова')).toBeInTheDocument()
  })
})

describe('session storage', () => {
  it('works in memory when the browser refuses storage', async () => {
    const { useSession: store } = await import('./session')
    const set = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('denied')
    })
    const get = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('denied')
    })
    const remove = vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => {
      throw new Error('denied')
    })
    store.getState().signIn(TOKEN, intern)
    expect(store.getState().token).toBe(TOKEN)
    await store.persist.rehydrate()
    store.persist.clearStorage()
    store.getState().signOut()
    expect(store.getState().token).toBeNull()
    for (const s of [set, get, remove]) s.mockRestore()
  })

  it('treats a gateway error as the server being down', async () => {
    server.use(http.post(`${API}/v1/public/auth/login`, () => new HttpResponse(null, { status: 502 })))
    renderApp('/login', { signedIn: false })
    await fill('intern', 'intern-demo')
    expect(await screen.findByRole('alert')).toHaveTextContent('Сервер не отвечает')
  })
})
