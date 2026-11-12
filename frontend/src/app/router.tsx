import { createRootRoute, createRoute, createRouter } from '@tanstack/react-router'
import type { RouterHistory } from '@tanstack/react-router'
import { Home } from '../features/home/Home'

// One screen: the route only says which orb is in focus, Home reads it from the location.
const rootRoute = createRootRoute({ component: Home })
const child = { getParentRoute: () => rootRoute }

const routeTree = rootRoute.addChildren([
  createRoute({ ...child, path: '/' }),
  createRoute({ ...child, path: '/today' }),
  createRoute({ ...child, path: '/plan' }),
  createRoute({ ...child, path: '/work' }),
  createRoute({ ...child, path: '/team' }),
])

export function createAppRouter(history?: RouterHistory) {
  return createRouter({ routeTree, history })
}

declare module '@tanstack/react-router' {
  interface Register {
    router: ReturnType<typeof createAppRouter>
  }
}
