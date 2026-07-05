import { createQueryOptions } from '@connectrpc/connect-query'
import { getSystemSettings, Permission } from '@moonbase/api-client'
import { createFileRoute } from '@tanstack/react-router'
import { EmailPanel } from '#components/system/email-panel'
import { requirePermission } from '#lib/session'
import { useSystemSettings } from '#lib/system-settings'

export const Route = createFileRoute('/_authed/settings/email')({
  beforeLoad: ({ context: { queryClient, transport } }) =>
    requirePermission(queryClient, transport, Permission.SYSTEM_READ),
  loader: ({ context: { queryClient, transport } }) =>
    queryClient.ensureQueryData(createQueryOptions(getSystemSettings, undefined, { transport })),
  component: EmailPage,
})

function EmailPage() {
  const { data, invalidate } = useSystemSettings()
  return <EmailPanel email={data.email} onChanged={invalidate} />
}
