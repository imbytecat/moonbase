import { CloudOutlined } from '@ant-design/icons'
import { useMutation, useQuery } from '@connectrpc/connect-query'
import {
  createSmsProfile,
  describeSmsProviders,
  type FieldDescriptor,
  type Profile,
  sendTestSms,
  updateSmsProfile,
} from '@moonbase/api-client'
import { App, Button, Form, Input, Select, Switch } from 'antd'
import { useState } from 'react'
import { PhoneInput } from '#components/phone-input'
import { ProfileFormDrawer, type ProviderOption } from '#components/profile-form-drawer'
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

function fieldControl(f: FieldDescriptor, secretPlaceholder: string) {
  if (f.secret)
    return <Input.Password autoComplete="new-password" placeholder={secretPlaceholder} />
  if (f.type === 'enum') return <Select options={f.options.map((o) => ({ value: o, label: o }))} />
  if (f.type === 'bool') return <Switch />
  if (f.type === 'text') return <Input.TextArea autoSize={{ minRows: 2 }} />
  return <Input autoComplete="off" placeholder={f.help} />
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
  const [form] = Form.useForm()
  const [result, setResult] = useState<TestState>()
  const [testPhone, setTestPhone] = useState('')

  const { data: describe } = useQuery(describeSmsProviders, {})
  const schemas = describe?.providers ?? {}

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

  const providers: ProviderOption[] = Object.keys(schemas).map((key) => ({
    value: key,
    label: PROVIDER_LABELS[key]?.() ?? key,
    description: PROVIDER_DESCS[key]?.() ?? '',
    icon: <CloudOutlined className="text-xl text-(--ant-color-primary)" />,
  }))

  const configOf = (provider: string): Record<string, string> => {
    const stored = profile?.provider === provider ? profile.config : undefined
    const out: Record<string, string> = {}
    for (const f of schemas[provider]?.fields ?? []) {
      out[f.key] = f.secret ? '' : String(stored?.[f.key] ?? '')
    }
    return out
  }

  const toProto = (
    provider: string,
    values: { name?: string; config?: Record<string, string> },
  ) => {
    const config: Record<string, string> = {}
    for (const f of schemas[provider]?.fields ?? []) config[f.key] = values.config?.[f.key] ?? ''
    return { id: profile?.id ?? '', name: values.name ?? '', provider, config }
  }

  return (
    <ProfileFormDrawer
      open={open}
      onClose={onClose}
      form={form}
      profileProvider={profile?.provider}
      providers={providers}
    >
      {(provider) => {
        const fields = schemas[provider]?.fields ?? []
        return (
          <Form
            form={form}
            layout="vertical"
            requiredMark={false}
            initialValues={{ name: profile?.name ?? '', config: configOf(provider) }}
            onFinish={(values) => {
              const p = toProto(provider, values)
              if (profile) updateMutation.mutate({ profile: p })
              else createMutation.mutate({ profile: p })
            }}
          >
            <Form.Item
              name="name"
              label={'配置名称'}
              rules={[{ required: true, message: '请输入配置名称' }]}
            >
              <Input placeholder={'如：国内通道、国际通道'} />
            </Form.Item>

            <div className="grid grid-cols-2 gap-4">
              {fields.map((f) => (
                <Form.Item
                  key={f.key}
                  name={['config', f.key]}
                  label={f.label}
                  rules={f.required && !f.secret ? [{ required: true }] : []}
                  valuePropName={f.type === 'bool' ? 'checked' : 'value'}
                >
                  {fieldControl(f, profile?.config?.[`${f.key}_set`] ? '留空保持不变' : '')}
                </Form.Item>
              ))}
            </div>

            <TestAlert result={result} />
            <div className="flex gap-2">
              <Button
                type="primary"
                htmlType="submit"
                loading={createMutation.isPending || updateMutation.isPending}
              >
                {'保存'}
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
                {'发送测试短信'}
              </Button>
            </div>
          </Form>
        )
      }}
    </ProfileFormDrawer>
  )
}
