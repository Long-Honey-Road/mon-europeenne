import '@testing-library/jest-dom/vitest'
import { cleanup, configure } from '@testing-library/react'
import { afterAll, afterEach, beforeAll } from 'vitest'
import { useSession } from '../features/auth/session'
import { server } from './server'
import { chat } from './chat'
import { resetPanels } from './panels'
import { requests } from './requests'
import { useTaskProgress } from '../features/task/progress'
import { usePlanHighlight } from '../features/plan/highlight'

// The chat loads threads, then history and its stream; coverage runs make that slower than the 1 s default.
configure({ asyncUtilTimeout: 3000 })

beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => {
  cleanup()
  server.resetHandlers()
  useSession.getState().signOut()
  localStorage.clear()
  chat.reset()
  resetPanels()
  requests.reset()
  useTaskProgress.getState().reset()
  usePlanHighlight.getState().set([], '')
})
afterAll(() => server.close())

// jsdom has no scrolling; the router restores scroll on navigation.
window.scrollTo = () => {}

// jsdom has no canvas; the scene simply skips drawing without a context.
HTMLCanvasElement.prototype.getContext = (() => null) as HTMLCanvasElement['getContext']
