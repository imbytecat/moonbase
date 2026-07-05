import { createFileRoute } from '@tanstack/react-router'
import { SettingsLayout } from '#components/settings-layout'
import { requireAnyPermission } from '#lib/session'
import { SETTINGS_PERMISSIONS } from '#lib/settings-nav'

export const Route = createFileRoute('/_authed/settings')({
  beforeLoad: ({ context: { queryClient, transport } }) =>
    requireAnyPermission(queryClient, transport, SETTINGS_PERMISSIONS),
  component: SettingsLayout,
})
