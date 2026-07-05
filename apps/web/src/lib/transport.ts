import { createConnectTransport } from '@connectrpc/connect-web'

// One transport per app. baseUrl '/api' matches the Vite dev proxy (/api ->
// :8080) and the same-origin prod embed; a mobile app would point this at the
// server's absolute URL instead. JSON (the connect-web default) keeps calls
// curl-able and readable in DevTools.
export const transport = createConnectTransport({
  baseUrl: '/api',
})
