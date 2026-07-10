import { CloudOutlined, MailOutlined } from '@ant-design/icons'
import { useMutation, useQuery } from '@connectrpc/connect-query'
import {
  createEmailProfile,
  describeEmailProviders,
  type Profile,
  sendTestEmail,
  updateEmailProfile,
} from '@moonbase/api-client'
import { App, Button, Input } from 'antd'
import { useState } from 'react'
import { ProfileFormDrawer, type ProviderOption } from '#components/profile-form-drawer'
import { ConfigForm } from '#components/system/config-form'
import { TestAlert, type TestState } from '#components/system/test-alert'
import { humanizeError } from '#lib/errors'

export function EmailProfileDrawer({
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
  const [testTo, setTestTo] = useState('')

  const { data: describe } = useQuery(describeEmailProviders, {})
  const forms = describe?.providers ?? {}

  const createMutation = useMutation(createEmailProfile, {
    onSuccess: () => {
      message.success('存储配置已创建')
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const updateMutation = useMutation(updateEmailProfile, {
    onSuccess: () => {
      message.success('设置已保存')
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const testMutation = useMutation(sendTestEmail, {
    onSuccess: (res) => setResult({ ok: res.ok, message: res.message }),
    onError: (err) => setResult({ ok: false, message: humanizeError(err) }),
  })

  const providers: ProviderOption[] = [
    {
      value: 'smtp',
      label: 'SMTP',
      description: '通用邮件发送协议，适配任何邮件服务商',
      icon: <MailOutlined className="text-xl text-(--ant-color-primary)" />,
    },
    {
      value: 'cloudflare',
      label: 'Cloudflare',
      description: 'Cloudflare 邮件发送接口',
      icon: <CloudOutlined className="text-xl text-(--ant-color-warning)" />,
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
              <>
                <Input
                  className="max-w-56"
                  placeholder={'测试收件邮箱'}
                  value={testTo}
                  onChange={(e) => setTestTo(e.target.value)}
                />
                <Button
                  loading={testMutation.isPending}
                  disabled={!testTo}
                  onClick={() => {
                    setResult(undefined)
                    testMutation.mutate({ to: testTo, profile: current })
                  }}
                >
                  {'发送测试邮件'}
                </Button>
              </>
            )}
          />
        )
      }}
    </ProfileFormDrawer>
  )
}
