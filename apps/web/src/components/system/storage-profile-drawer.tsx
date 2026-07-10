import { CloudServerOutlined, HddOutlined } from '@ant-design/icons'
import { useMutation, useQuery } from '@connectrpc/connect-query'
import {
  createStorageProfile,
  describeStorageProviders,
  type Profile,
  testStorageConnection,
  updateStorageProfile,
} from '@moonbase/api-client'
import { App, Button } from 'antd'
import { useState } from 'react'
import { ProfileFormDrawer, type ProviderOption } from '#components/profile-form-drawer'
import { ConfigForm } from '#components/system/config-form'
import { TestAlert, type TestState } from '#components/system/test-alert'
import { humanizeError } from '#lib/errors'

export function StorageProfileDrawer({
  profile,
  open,
  onClose,
  onChanged,
}: {
  profile: Profile | undefined
  open: boolean
  onClose: () => void
  onChanged: () => void
}) {
  const { message } = App.useApp()
  const [dirty, setDirty] = useState(false)
  const [result, setResult] = useState<TestState>()

  const { data: describe } = useQuery(describeStorageProviders, {})
  const forms = describe?.providers ?? {}

  const createMutation = useMutation(createStorageProfile, {
    onSuccess: () => {
      message.success('存储配置已创建')
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const updateMutation = useMutation(updateStorageProfile, {
    onSuccess: () => {
      message.success('设置已保存')
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const testMutation = useMutation(testStorageConnection, {
    onSuccess: (res) => setResult({ ok: res.ok, message: res.message }),
    onError: (err) => setResult({ ok: false, message: humanizeError(err) }),
  })

  const providers: ProviderOption[] = [
    {
      value: 'local',
      label: '本地存储',
      description: '文件保存在服务器磁盘上，零外部依赖，适合单机部署',
      icon: <HddOutlined className="text-xl text-(--ant-color-primary)" />,
    },
    {
      value: 's3',
      label: 'S3 兼容存储',
      description: '任何 S3 兼容服务：AWS S3、MinIO、R2 等',
      icon: <CloudServerOutlined className="text-xl text-(--ant-color-warning)" />,
    },
  ]

  return (
    <ProfileFormDrawer
      open={open}
      onClose={onClose}
      dirty={dirty}
      profileProvider={profile?.provider}
      providers={providers}
    >
      {(provider) => {
        const providerForm = forms[provider]
        if (!providerForm) return null
        return (
          <ConfigForm
            key={provider}
            providerForm={providerForm}
            provider={provider}
            profile={profile}
            saving={createMutation.isPending || updateMutation.isPending}
            onDirtyChange={setDirty}
            onSubmit={(p) => {
              if (profile) updateMutation.mutate({ profile: p })
              else createMutation.mutate({ profile: p })
            }}
            banner={() => <TestAlert result={result} />}
            actions={(current) => (
              <Button
                loading={testMutation.isPending}
                onClick={() => {
                  setResult(undefined)
                  testMutation.mutate({ profile: current })
                }}
              >
                {'测试连接'}
              </Button>
            )}
          />
        )
      }}
    </ProfileFormDrawer>
  )
}
