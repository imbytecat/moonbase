import { createFileRoute, redirect } from '@tanstack/react-router'
import { ensureSession } from '#lib/session'
import { firstSettingsPath } from '#lib/settings-nav'

export const Route = createFileRoute('/_authed/settings/')({
  beforeLoad: async ({ context: { queryClient, transport } }) => {
    const user = await ensureSession(queryClient, transport)
    const path = firstSettingsPath(user)
    if (path) throw redirect({ to: path, replace: true })
  },
})
