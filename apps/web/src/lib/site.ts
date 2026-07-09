import type { Transport } from '@connectrpc/connect'
import { createConnectQueryKey, createQueryOptions } from '@connectrpc/connect-query'
import { type GetSiteInfoResponse, getSiteInfo } from '@moonbase/api-client'
import type { QueryClient } from '@tanstack/react-query'

// Site identity is public data (GetSiteInfo has no auth), loaded once in the
// root route and cached; every branded surface (login card, sidebar, document
// head) derives from this query instead of hardcoding the product name.

export function siteInfoQueryOptions(transport: Transport) {
  return createQueryOptions(getSiteInfo, undefined, { transport })
}

export function siteInfoQueryKey() {
  return createConnectQueryKey({ schema: getSiteInfo, cardinality: 'finite' })
}

export function siteName(info: GetSiteInfoResponse | undefined): string {
  return info?.name || 'Moonbase'
}

// Syncs the document head with the configured identity: tab title and
// favicon. Runs on load and whenever site settings change.
export function applySiteInfoToDocument(info: GetSiteInfoResponse | undefined) {
  document.title = siteName(info)
  const href = info?.faviconUrl
  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!href) {
    link?.remove()
    return
  }
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }
  link.href = href
}

export async function prefetchSiteInfo(queryClient: QueryClient, transport: Transport) {
  try {
    return await queryClient.ensureQueryData(siteInfoQueryOptions(transport))
  } catch {
    return undefined
  }
}
