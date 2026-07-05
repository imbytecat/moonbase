import type { Transport } from '@connectrpc/connect'
import { useQuery } from '@connectrpc/connect-query'
import { getSiteInfo } from '@moonbase/api-client'
import type { QueryClient } from '@tanstack/react-query'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import { createRootRouteWithContext, Outlet } from '@tanstack/react-router'
import { TanStackRouterDevtools } from '@tanstack/react-router-devtools'
import { useEffect } from 'react'
import { applySiteInfoToDocument, prefetchSiteInfo } from '#lib/site'

export interface RouterContext {
  queryClient: QueryClient
  transport: Transport
}

// The root is chrome-free: /login and /register render standalone, while every
// app page lives under the _authed pathless layout (guard + admin shell).
// Site identity (name/favicon) is public data prefetched here so the document
// head is branded before any child renders; a failed fetch falls back to the
// built-in name and never blocks the app.
export const Route = createRootRouteWithContext<RouterContext>()({
  loader: ({ context: { queryClient, transport } }) => prefetchSiteInfo(queryClient, transport),
  component: RootLayout,
})

function RootLayout() {
  const { data: siteInfo } = useQuery(getSiteInfo)

  useEffect(() => {
    applySiteInfoToDocument(siteInfo)
  }, [siteInfo])

  return (
    <>
      <Outlet />
      {import.meta.env.DEV ? (
        <>
          <TanStackRouterDevtools position="bottom-right" />
          <ReactQueryDevtools initialIsOpen={false} buttonPosition="bottom-left" />
        </>
      ) : null}
    </>
  )
}
