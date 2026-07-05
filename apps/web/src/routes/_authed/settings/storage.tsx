import { createQueryOptions } from '@connectrpc/connect-query'
import { getSystemSettings, Permission } from '@moonbase/api-client'
import { createFileRoute } from '@tanstack/react-router'
import { StoragePanel } from '#components/system/storage-panel'
import { requirePermission } from '#lib/session'
import { useSystemSettings } from '#lib/system-settings'

export const Route = createFileRoute('/_authed/settings/storage')({
  beforeLoad: ({ context: { queryClient, transport } }) =>
    requirePermission(queryClient, transport, Permission.SYSTEM_READ),
  loader: ({ context: { queryClient, transport } }) =>
    queryClient.ensureQueryData(createQueryOptions(getSystemSettings, undefined, { transport })),
  component: StoragePage,
})

function StoragePage() {
  const { data, invalidate } = useSystemSettings()
  return <StoragePanel storage={data.storage} onChanged={invalidate} />
}
