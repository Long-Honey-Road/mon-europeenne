import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterAll, afterEach, beforeAll } from 'vitest'
import { useSession } from '../features/auth/session'
import { server } from './api'

beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => {
  cleanup()
  server.resetHandlers()
  useSession.getState().signOut()
  localStorage.clear()
})
afterAll(() => server.close())

// jsdom has no scrolling; the router restores scroll on navigation.
window.scrollTo = () => {}

// jsdom has no canvas; the scene simply skips drawing without a context.
HTMLCanvasElement.prototype.getContext = (() => null) as HTMLCanvasElement['getContext']
