import { render } from '@testing-library/react'
import { RouterProvider, createMemoryHistory } from '@tanstack/react-router'
import { AppProviders, createQueryClient } from '../app/providers'
import { createAppRouter } from '../app/router'
import { useSession } from '../features/auth/session'
import { TOKEN, intern } from './api'

/** Renders the whole app at `path`, signed in as the test intern unless `signedIn` is false. */
export function renderApp(path: string, { signedIn = true } = {}) {
  if (signedIn) useSession.getState().signIn(TOKEN, intern)
  const router = createAppRouter(createMemoryHistory({ initialEntries: [path] }))
  const queryClient = createQueryClient()
  queryClient.setDefaultOptions({ queries: { retry: false } })
  render(
    <AppProviders client={queryClient}>
      <RouterProvider router={router} />
    </AppProviders>,
  )
  return { router, queryClient }
}
