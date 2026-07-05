import { createQueryOptions } from '@connectrpc/connect-query'
import { getSystemSettings, Permission } from '@moonbase/api-client'
import { createFileRoute } from '@tanstack/react-router'
import { OauthPanel } from '#components/system/oauth-panel'
import { requirePermission } from '#lib/session'
import { useSystemSettings } from '#lib/system-settings'

export const Route = createFileRoute('/_authed/settings/oauth')({
  beforeLoad: ({ context: { queryClient, transport } }) =>
    requirePermission(queryClient, transport, Permission.SYSTEM_READ),
  loader: ({ context: { queryClient, transport } }) =>
    queryClient.ensureQueryData(createQueryOptions(getSystemSettings, undefined, { transport })),
  component: OauthPage,
})

function OauthPage() {
  const { data, invalidate } = useSystemSettings()
  return <OauthPanel oauth={data.oauth} onChanged={invalidate} />
}
