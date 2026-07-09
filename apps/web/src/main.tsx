import { TransportProvider } from '@connectrpc/connect-query'
import { QueryClientProvider } from '@tanstack/react-query'
import { createRouter, RouterProvider } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouteError, RouteNotFound, RoutePending } from './components/route-fallbacks'
import { queryClient } from './lib/query-client'
import { transport } from './lib/transport'
import { AppProviders } from './providers/app-providers'
import { applyInitialTheme } from './providers/theme-mode'
import { routeTree } from './routeTree.gen'
import './styles.css'

document.documentElement.lang = 'zh-CN'
applyInitialTheme()

const router = createRouter({
  routeTree,
  context: { queryClient, transport },
  defaultPreload: 'intent',
  // Loaders delegate caching to TanStack Query (ensureQueryData), so the
  // router's own loader cache would only mask query invalidations.
  defaultPreloadStaleTime: 0,
  defaultErrorComponent: RouteError,
  defaultNotFoundComponent: RouteNotFound,
  defaultPendingComponent: RoutePending,
  // Show the skeleton quickly on slow loads, but once shown keep it long
  // enough to avoid a single-frame flash on fast ones.
  defaultPendingMs: 150,
  defaultPendingMinMs: 300,
  scrollRestoration: true,
  // Navigations cross-fade via the browser's View Transitions API; browsers
  // without it (and reduced-motion users, see styles.css) just skip the effect.
  defaultViewTransition: true,
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

const rootElement = document.getElementById('root')
if (!rootElement) throw new Error('Root element #root not found')

createRoot(rootElement).render(
  <StrictMode>
    <TransportProvider transport={transport}>
      <QueryClientProvider client={queryClient}>
        <AppProviders>
          <RouterProvider router={router} />
        </AppProviders>
      </QueryClientProvider>
    </TransportProvider>
  </StrictMode>,
)
