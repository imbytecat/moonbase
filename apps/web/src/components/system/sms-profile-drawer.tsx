import { CloudOutlined } from '@ant-design/icons'
import { useMutation } from '@connectrpc/connect-query'
import {
  createSmsProfile,
  type SmsProfile,
  sendTestSms,
  updateSmsProfile,
} from '@moonbase/api-client'
import { App, Button, Form, Input } from 'antd'
import { useState } from 'react'
import { PhoneInput } from '#components/phone-input'
import { ProfileFormDrawer, type ProviderOption } from '#components/profile-form-drawer'
import { TestAlert, type TestState } from '#components/system/test-alert'
import { humanizeError } from '#lib/errors'
import { m } from '#paraglide/messages.js'

interface SmsProfileFormValues {
  name: string
  aliyun: { accessKeyId: string; accessKeySecret: string; signName: string; templateCode: string }
  tencent: {
    secretId: string
    secretKey: string
    sdkAppId: string
    signName: string
    templateId: string
    region: string
  }
}

export function SmsProfileDrawer({
  profile,
  open,
  onClose,
  onChanged,
}: {
  profile: SmsProfile | undefined
  open: boolean
  onClose: () => void
  onChanged: () => void
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<SmsProfileFormValues>()
  const [result, setResult] = useState<TestState>()
  const [testPhone, setTestPhone] = useState('')

  const createMutation = useMutation(createSmsProfile, {
    onSuccess: () => {
      message.success(m.systemPage_profileCreated())
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const updateMutation = useMutation(updateSmsProfile, {
    onSuccess: () => {
      message.success(m.systemPage_saved())
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const testMutation = useMutation(sendTestSms, {
    onSuccess: (res) => setResult({ ok: res.ok, message: res.message }),
    onError: (err) => setResult({ ok: false, message: humanizeError(err) }),
  })

  const providers: ProviderOption[] = [
    {
      value: 'aliyun',
      label: m.systemPage_providerAliyun(),
      description: m.systemPage_aliyunSmsDesc(),
      icon: <CloudOutlined className="text-xl text-(--ant-color-warning)" />,
    },
    {
      value: 'tencent',
      label: m.systemPage_providerTencent(),
      description: m.systemPage_tencentSmsDesc(),
      icon: <CloudOutlined className="text-xl text-(--ant-color-primary)" />,
    },
  ]

  const storedAliyun = () => ({
    accessKeyId: profile?.aliyun?.accessKeyId ?? '',
    accessKeySecret: '',
    signName: profile?.aliyun?.signName ?? '',
    templateCode: profile?.aliyun?.templateCode ?? '',
  })
  const storedTencent = () => ({
    secretId: profile?.tencent?.secretId ?? '',
    secretKey: '',
    sdkAppId: profile?.tencent?.sdkAppId ?? '',
    signName: profile?.tencent?.signName ?? '',
    templateId: profile?.tencent?.templateId ?? '',
    region: profile?.tencent?.region ?? '',
  })

  const toProto = (provider: string, values: SmsProfileFormValues) => ({
    id: profile?.id ?? '',
    name: values.name ?? '',
    provider,
    aliyun: provider === 'aliyun' ? values.aliyun : storedAliyun(),
    tencent: provider === 'tencent' ? values.tencent : storedTencent(),
  })

  const aliyunSecretPlaceholder = profile?.aliyun?.accessKeySecretSet
    ? m.systemPage_secretUnchanged()
    : ''
  const tencentSecretPlaceholder = profile?.tencent?.secretKeySet
    ? m.systemPage_secretUnchanged()
    : ''

  return (
    <ProfileFormDrawer
      open={open}
      onClose={onClose}
      form={form}
      profileProvider={profile?.provider}
      providers={providers}
    >
      {(provider) => (
        <Form
          form={form}
          layout="vertical"
          requiredMark={false}
          initialValues={{
            name: profile?.name ?? '',
            aliyun: storedAliyun(),
            tencent: storedTencent(),
          }}
          onFinish={(values) => {
            const p = toProto(provider, values)
            if (profile) updateMutation.mutate({ profile: p })
            else createMutation.mutate({ profile: p })
          }}
        >
          <Form.Item
            name="name"
            label={m.systemPage_profileName()}
            rules={[{ required: true, message: m.systemPage_profileNameRule() }]}
          >
            <Input placeholder={m.systemPage_smsProfileNamePlaceholder()} />
          </Form.Item>

          {provider === 'aliyun' ? (
            <div className="grid grid-cols-2 gap-4">
              <Form.Item name={['aliyun', 'accessKeyId']} label={m.systemPage_accessKeyId()}>
                <Input autoComplete="off" />
              </Form.Item>
              <Form.Item
                name={['aliyun', 'accessKeySecret']}
                label={m.systemPage_accessKeySecret()}
              >
                <Input.Password autoComplete="new-password" placeholder={aliyunSecretPlaceholder} />
              </Form.Item>
              <Form.Item name={['aliyun', 'signName']} label={m.systemPage_signName()}>
                <Input />
              </Form.Item>
              <Form.Item name={['aliyun', 'templateCode']} label={m.systemPage_templateCode()}>
                <Input placeholder="SMS_123456789" />
              </Form.Item>
            </div>
          ) : (
            <div className="grid grid-cols-2 gap-4">
              <Form.Item name={['tencent', 'secretId']} label="SecretId">
                <Input autoComplete="off" />
              </Form.Item>
              <Form.Item name={['tencent', 'secretKey']} label="SecretKey">
                <Input.Password
                  autoComplete="new-password"
                  placeholder={tencentSecretPlaceholder}
                />
              </Form.Item>
              <Form.Item name={['tencent', 'sdkAppId']} label="SdkAppId">
                <Input autoComplete="off" />
              </Form.Item>
              <Form.Item name={['tencent', 'signName']} label={m.systemPage_signName()}>
                <Input />
              </Form.Item>
              <Form.Item name={['tencent', 'templateId']} label={m.systemPage_templateId()}>
                <Input />
              </Form.Item>
              <Form.Item name={['tencent', 'region']} label={m.systemPage_region()}>
                <Input placeholder="ap-guangzhou" />
              </Form.Item>
            </div>
          )}

          <TestAlert result={result} />
          <div className="flex gap-2">
            <Button
              type="primary"
              htmlType="submit"
              loading={createMutation.isPending || updateMutation.isPending}
            >
              {m.common_save()}
            </Button>
            <div className="flex-1">
              <PhoneInput allowedRegions={[]} value={testPhone} onChange={setTestPhone} />
            </div>
            <Button
              loading={testMutation.isPending}
              disabled={!testPhone}
              onClick={() => {
                setResult(undefined)
                testMutation.mutate({
                  phoneNumber: testPhone,
                  profile: toProto(provider, form.getFieldsValue()),
                })
              }}
            >
              {m.systemPage_sendTestSms()}
            </Button>
          </div>
        </Form>
      )}
    </ProfileFormDrawer>
  )
}
