import { createQueryOptions, useSuspenseQuery } from '@connectrpc/connect-query'
import { getSettings, Permission } from '@moonbase/api-client'
import { createFileRoute } from '@tanstack/react-router'
import { SitePanel } from '#components/settings/site-panel'
import { useUpdateSettings } from '#lib/business-settings'
import { requirePermission } from '#lib/session'

export const Route = createFileRoute('/_authed/settings/site')({
  beforeLoad: ({ context: { queryClient, transport } }) =>
    requirePermission(queryClient, transport, Permission.SETTINGS_READ),
  loader: ({ context: { queryClient, transport } }) =>
    queryClient.ensureQueryData(createQueryOptions(getSettings, undefined, { transport })),
  component: SitePage,
})

function SitePage() {
  const { data } = useSuspenseQuery(getSettings)
  const updateMutation = useUpdateSettings()

  return (
    <SitePanel
      site={data.site}
      saving={updateMutation.isPending}
      onSave={(site) => updateMutation.mutate({ site })}
    />
  )
}
