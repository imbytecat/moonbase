import { createQueryOptions, useQuery, useSuspenseQuery } from '@connectrpc/connect-query'
import { getAuthConfig, getSettings, Permission } from '@moonbase/api-client'
import { createFileRoute } from '@tanstack/react-router'
import { Select, Switch, Typography } from 'antd'
import { useUpdateSettings } from '#lib/business-settings'
import { requirePermission } from '#lib/session'

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
          <Typography.Text strong>{'开放注册'}</Typography.Text>
          <div className="text-xs text-(--ant-color-text-tertiary)">{'允许新用户自助注册'}</div>
        </div>
        <Switch
          checked={data.auth?.registrationEnabled}
          loading={updateMutation.isPending}
          onChange={(checked) => saveAuth({ registrationEnabled: checked })}
        />
      </div>

      <div>
        <Typography.Text strong>{'注册收集的标识'}</Typography.Text>
        <div className="mb-2 text-xs text-(--ant-color-text-tertiary)">
          {'注册表单需要用户填写哪些标识，至少保留一项；邮箱和手机号注册时会验证码验证所有权'}
        </div>
        <Select
          mode="multiple"
          className="w-full"
          value={identifiers}
          options={[
            { value: 'username', label: '用户名' },
            {
              value: 'email',
              label: '邮箱',
              disabled: !authConfig?.emailEnabled,
            },
            {
              value: 'phone',
              label: '手机号',
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
            {'邮箱注册需要先配置可用的邮件通道'}
          </div>
        )}
        {authConfig?.smsEnabled ? null : (
          <div className="mt-1 text-xs text-(--ant-color-text-tertiary)">
            {'手机号注册需要先配置可用的短信通道'}
          </div>
        )}
      </div>
    </div>
  )
}
