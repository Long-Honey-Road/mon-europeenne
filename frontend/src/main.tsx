import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider } from '@tanstack/react-router'
import '@fontsource-variable/manrope'
import './shared/styles/tokens.css'
import './shared/styles/global.css'
import { AppProviders, createQueryClient } from './app/providers'
import { createAppRouter } from './app/router'

const router = createAppRouter()
const queryClient = createQueryClient()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <AppProviders client={queryClient}>
      <RouterProvider router={router} />
    </AppProviders>
  </StrictMode>,
)
