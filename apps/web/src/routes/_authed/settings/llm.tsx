import { createQueryOptions } from '@connectrpc/connect-query'
import { getSystemSettings, Permission } from '@moonbase/api-client'
import { createFileRoute } from '@tanstack/react-router'
import { LlmPanel } from '#components/system/llm-panel'
import { requirePermission } from '#lib/session'
import { useSystemSettings } from '#lib/system-settings'

export const Route = createFileRoute('/_authed/settings/llm')({
  beforeLoad: ({ context: { queryClient, transport } }) =>
    requirePermission(queryClient, transport, Permission.SYSTEM_READ),
  loader: ({ context: { queryClient, transport } }) =>
    queryClient.ensureQueryData(createQueryOptions(getSystemSettings, undefined, { transport })),
  component: LlmPage,
})

function LlmPage() {
  const { data, invalidate } = useSystemSettings()
  return <LlmPanel llm={data.llm} onChanged={invalidate} />
}
