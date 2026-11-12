import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider } from '@tanstack/react-router'
import '@fontsource-variable/manrope'
import './shared/styles/tokens.css'
import './shared/styles/global.css'
import { createAppRouter } from './app/router'

const router = createAppRouter()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <RouterProvider router={router} />
  </StrictMode>,
)
