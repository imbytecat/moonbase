import { createQueryOptions } from '@connectrpc/connect-query'
import { getSystemSettings, Permission } from '@moonbase/api-client'
import { createFileRoute } from '@tanstack/react-router'
import { CaptchaPanel } from '#components/system/captcha-panel'
import { requirePermission } from '#lib/session'
import { useSystemSettings } from '#lib/system-settings'

export const Route = createFileRoute('/_authed/settings/captcha')({
  beforeLoad: ({ context: { queryClient, transport } }) =>
    requirePermission(queryClient, transport, Permission.SYSTEM_READ),
  loader: ({ context: { queryClient, transport } }) =>
    queryClient.ensureQueryData(createQueryOptions(getSystemSettings, undefined, { transport })),
  component: CaptchaPage,
})

function CaptchaPage() {
  const { data, invalidate } = useSystemSettings()
  return <CaptchaPanel captcha={data.captcha} onChanged={invalidate} />
}
