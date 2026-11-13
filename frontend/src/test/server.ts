import { setupServer } from 'msw/node'
import { handlers } from './api'

/** The fake API for tests in Node. */
export const server = setupServer(...handlers)
