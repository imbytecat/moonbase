import { CloudOutlined } from '@ant-design/icons'
import { useMutation, useQuery } from '@connectrpc/connect-query'
import {
  createSmsProfile,
  describeSmsProviders,
  type Profile,
  sendTestSms,
  updateSmsProfile,
} from '@moonbase/api-client'
import { App, Button } from 'antd'
import { useState } from 'react'
import { PhoneInput } from '#components/phone-input'
import { ProfileFormDrawer, type ProviderOption } from '#components/profile-form-drawer'
import { ConfigForm } from '#components/system/config-form'
import { TestAlert, type TestState } from '#components/system/test-alert'
import { humanizeError } from '#lib/errors'

const PROVIDER_LABELS: Record<string, () => string> = {
  aliyun: () => '阿里云短信',
  tencent: () => '腾讯云短信',
}
const PROVIDER_DESCS: Record<string, () => string> = {
  aliyun: () => '阿里云短信服务',
  tencent: () => '腾讯云短信服务',
}

export function SmsProfileDrawer({
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
  const [testPhone, setTestPhone] = useState('')

  const { data: describe } = useQuery(describeSmsProviders, {})
  const forms = describe?.providers ?? {}

  const createMutation = useMutation(createSmsProfile, {
    onSuccess: () => {
      message.success('存储配置已创建')
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const updateMutation = useMutation(updateSmsProfile, {
    onSuccess: () => {
      message.success('设置已保存')
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const testMutation = useMutation(sendTestSms, {
    onSuccess: (res) => setResult({ ok: res.ok, message: res.message }),
    onError: (err) => setResult({ ok: false, message: humanizeError(err) }),
  })

  const providers: ProviderOption[] = Object.keys(forms).map((key) => ({
    value: key,
    label: PROVIDER_LABELS[key]?.() ?? key,
    description: PROVIDER_DESCS[key]?.() ?? '',
    icon: <CloudOutlined className="text-xl text-(--ant-color-primary)" />,
  }))

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
                <div className="flex-1">
                  <PhoneInput allowedRegions={[]} value={testPhone} onChange={setTestPhone} />
                </div>
                <Button
                  loading={testMutation.isPending}
                  disabled={!testPhone}
                  onClick={() => {
                    setResult(undefined)
                    testMutation.mutate({ phoneNumber: testPhone, profile: current })
                  }}
                >
                  {'发送测试短信'}
                </Button>
              </>
            )}
          />
        )
      }}
    </ProfileFormDrawer>
  )
}
