import { createQueryOptions, useQuery, useSuspenseQuery } from '@connectrpc/connect-query'
import { getAuthConfig, getSettings, Permission } from '@moonbase/api-client'
import { createFileRoute } from '@tanstack/react-router'
import { Select, Switch, Typography } from 'antd'
import { useUpdateSettings } from '#lib/business-settings'
import { requirePermission } from '#lib/session'
import { m } from '#paraglide/messages.js'

export const Route = createFileRoute('/_authed/settings/registration')({
  beforeLoad: ({ context: { queryClient, transport } }) =>
    requirePermission(queryClient, transport, Permission.SETTINGS_READ),
  loader: ({ context: { queryClient, transport } }) =>
    queryClient.ensureQueryData(createQueryOptions(getSettings, undefined, { transport })),
  component: RegistrationPage,
})

function RegistrationPage() {
  const { data } = useSuspenseQuery(getSettings)
  const { data: authConfig } = useQuery(getAuthConfig)
  const updateMutation = useUpdateSettings()

  const saveAuth = (
    patch: Partial<{
      registrationEnabled: boolean
      signupIdentifiers: string[]
    }>,
  ) =>
    updateMutation.mutate({
      auth: {
        registrationEnabled: data.auth?.registrationEnabled ?? false,
        allowedPhoneRegions: data.auth?.allowedPhoneRegions ?? [],
        signupIdentifiers: data.auth?.signupIdentifiers ?? ['username'],
        ...patch,
      },
    })

  const identifiers = data.auth?.signupIdentifiers ?? ['username']

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-4">
        <div>
          <Typography.Text strong>{m.settingsPage_openRegistration()}</Typography.Text>
          <div className="text-xs text-(--ant-color-text-tertiary)">
            {m.settingsPage_openRegistrationHint()}
          </div>
        </div>
        <Switch
          checked={data.auth?.registrationEnabled}
          loading={updateMutation.isPending}
          onChange={(checked) => saveAuth({ registrationEnabled: checked })}
        />
      </div>

      <div>
        <Typography.Text strong>{m.settingsPage_signupIdentifiers()}</Typography.Text>
        <div className="mb-2 text-xs text-(--ant-color-text-tertiary)">
          {m.settingsPage_signupIdentifiersHint()}
        </div>
        <Select
          mode="multiple"
          className="w-full"
          value={identifiers}
          options={[
            { value: 'username', label: m.auth_username() },
            {
              value: 'email',
              label: m.auth_email(),
              disabled: !authConfig?.emailEnabled,
            },
            {
              value: 'phone',
              label: m.auth_phone(),
              disabled: !authConfig?.smsEnabled,
            },
          ]}
          onChange={(values: string[]) => {
            if (values.length === 0) return
            saveAuth({ signupIdentifiers: values })
          }}
        />
        {authConfig?.emailEnabled ? null : (
          <div className="mt-1 text-xs text-(--ant-color-text-tertiary)">
            {m.settingsPage_emailNeedsChannel()}
          </div>
        )}
        {authConfig?.smsEnabled ? null : (
          <div className="mt-1 text-xs text-(--ant-color-text-tertiary)">
            {m.settingsPage_phoneNeedsSms()}
          </div>
        )}
      </div>
    </div>
  )
}
