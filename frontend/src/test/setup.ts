import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

afterEach(cleanup)

// jsdom has no scrolling; the router restores scroll on navigation.
window.scrollTo = () => {}

// jsdom has no canvas; the scene simply skips drawing without a context.
HTMLCanvasElement.prototype.getContext = (() => null) as HTMLCanvasElement['getContext']
