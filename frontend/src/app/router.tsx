import { Navigate, Outlet, createRootRoute, createRoute, createRouter, redirect } from '@tanstack/react-router'
import type { RouterHistory } from '@tanstack/react-router'
import { LoginPage } from '../features/auth/LoginPage'
import { useSession } from '../features/auth/session'
import { Home } from '../features/home/Home'

const rootRoute = createRootRoute({ component: Outlet })

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  validateSearch: (search: Record<string, unknown>) =>
    typeof search.redirect === 'string' ? { redirect: search.redirect } : {},
  beforeLoad: () => {
    if (useSession.getState().token) throw redirect({ to: '/' })
  },
  component: LoginPage,
})

// Leaves the app when the session ends mid-visit (sign-out or a 401 from the API).
function Guarded() {
  const token = useSession((s) => s.token)
  return token ? <Home /> : <Navigate to="/login" />
}

// One screen: the route only says which orb is in focus, Home reads it from the location.
const appRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: 'app',
  beforeLoad: ({ location }) => {
    if (!useSession.getState().token) {
      throw redirect({ to: '/login', search: { redirect: location.href } })
    }
  },
  component: Guarded,
})
const child = { getParentRoute: () => appRoute }

const routeTree = rootRoute.addChildren([
  loginRoute,
  appRoute.addChildren([
    createRoute({ ...child, path: '/' }),
    createRoute({ ...child, path: '/today' }),
    createRoute({ ...child, path: '/plan' }),
    createRoute({ ...child, path: '/plan/map' }),
    createRoute({ ...child, path: '/work' }),
    createRoute({ ...child, path: '/team' }),
    createRoute({ ...child, path: '/task/$threadId' }),
  ]),
])

export function createAppRouter(history?: RouterHistory) {
  return createRouter({ routeTree, history })
}

declare module '@tanstack/react-router' {
  interface Register {
    router: ReturnType<typeof createAppRouter>
  }
}
