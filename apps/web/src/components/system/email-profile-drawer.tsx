import { CloudOutlined, MailOutlined } from '@ant-design/icons'
import { useMutation } from '@connectrpc/connect-query'
import {
  createEmailProfile,
  type EmailProfile,
  sendTestEmail,
  updateEmailProfile,
} from '@moonbase/api-client'
import { App, Button, Form, Input, InputNumber, Select } from 'antd'
import { useState } from 'react'
import { ProfileFormDrawer, type ProviderOption } from '#components/profile-form-drawer'
import { TestAlert, type TestState } from '#components/system/test-alert'
import { humanizeError } from '#lib/errors'
import { m } from '#paraglide/messages.js'

interface EmailProfileFormValues {
  name: string
  fromAddress: string
  fromName: string
  smtp: {
    host: string
    port: number | null
    username: string
    password: string
    encryption: string
  }
  cloudflare: { accountId: string; apiToken: string }
}

export function EmailProfileDrawer({
  profile,
  open,
  onClose,
  onChanged,
}: {
  profile: EmailProfile | undefined
  open: boolean
  onClose: () => void
  onChanged: () => void
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<EmailProfileFormValues>()
  const [result, setResult] = useState<TestState>()
  const [testTo, setTestTo] = useState('')

  const createMutation = useMutation(createEmailProfile, {
    onSuccess: () => {
      message.success(m.systemPage_profileCreated())
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const updateMutation = useMutation(updateEmailProfile, {
    onSuccess: () => {
      message.success(m.systemPage_saved())
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
      description: m.systemPage_smtpDesc(),
      icon: <MailOutlined className="text-xl text-(--ant-color-primary)" />,
    },
    {
      value: 'cloudflare',
      label: 'Cloudflare',
      description: m.systemPage_cloudflareEmailDesc(),
      icon: <CloudOutlined className="text-xl text-(--ant-color-warning)" />,
    },
  ]

  const storedSmtp = () => ({
    host: profile?.smtp?.host ?? '',
    port: profile?.smtp?.port || 587,
    username: profile?.smtp?.username ?? '',
    password: '',
    encryption: profile?.smtp?.encryption || 'starttls',
  })
  const storedCloudflare = () => ({
    accountId: profile?.cloudflare?.accountId ?? '',
    apiToken: '',
  })

  const toProto = (provider: string, values: EmailProfileFormValues) => ({
    id: profile?.id ?? '',
    name: values.name ?? '',
    provider,
    fromAddress: values.fromAddress ?? '',
    fromName: values.fromName ?? '',
    smtp:
      provider === 'smtp'
        ? { ...storedSmtp(), ...values.smtp, port: values.smtp?.port ?? 587 }
        : storedSmtp(),
    cloudflare:
      provider === 'cloudflare'
        ? { ...storedCloudflare(), ...values.cloudflare }
        : storedCloudflare(),
  })

  const smtpSecretPlaceholder = profile?.smtp?.passwordSet ? m.systemPage_secretUnchanged() : ''
  const cfSecretPlaceholder = profile?.cloudflare?.apiTokenSet ? m.systemPage_secretUnchanged() : ''

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
            fromAddress: profile?.fromAddress ?? '',
            fromName: profile?.fromName ?? '',
            smtp: storedSmtp(),
            cloudflare: storedCloudflare(),
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
            <Input placeholder={m.systemPage_emailProfileNamePlaceholder()} />
          </Form.Item>
          <div className="grid grid-cols-2 gap-4">
            <Form.Item name="fromAddress" label={m.systemPage_fromAddress()}>
              <Input placeholder="noreply@example.com" />
            </Form.Item>
            <Form.Item name="fromName" label={m.systemPage_fromName()}>
              <Input />
            </Form.Item>
          </div>

          {provider === 'smtp' ? (
            <>
              <div className="grid grid-cols-2 gap-4">
                <Form.Item name={['smtp', 'host']} label={m.systemPage_smtpHost()}>
                  <Input placeholder="smtp.example.com" />
                </Form.Item>
                <Form.Item name={['smtp', 'port']} label={m.systemPage_port()}>
                  <InputNumber min={1} max={65535} className="!w-full" />
                </Form.Item>
                <Form.Item name={['smtp', 'username']} label={m.systemPage_username()}>
                  <Input autoComplete="off" />
                </Form.Item>
                <Form.Item name={['smtp', 'password']} label={m.systemPage_smtpPassword()}>
                  <Input.Password autoComplete="new-password" placeholder={smtpSecretPlaceholder} />
                </Form.Item>
              </div>
              <Form.Item name={['smtp', 'encryption']} label={m.systemPage_encryption()}>
                <Select
                  options={[
                    { label: 'STARTTLS (587)', value: 'starttls' },
                    { label: 'SSL/TLS (465)', value: 'ssl' },
                    { label: m.systemPage_encryptionNone(), value: 'none' },
                  ]}
                />
              </Form.Item>
            </>
          ) : (
            <div className="grid grid-cols-2 gap-4">
              <Form.Item name={['cloudflare', 'accountId']} label={m.systemPage_cfAccountId()}>
                <Input autoComplete="off" />
              </Form.Item>
              <Form.Item name={['cloudflare', 'apiToken']} label={m.systemPage_cfApiToken()}>
                <Input.Password autoComplete="new-password" placeholder={cfSecretPlaceholder} />
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
            <Input
              className="max-w-56"
              placeholder={m.systemPage_testRecipient()}
              value={testTo}
              onChange={(e) => setTestTo(e.target.value)}
            />
            <Button
              loading={testMutation.isPending}
              disabled={!testTo}
              onClick={() => {
                setResult(undefined)
                testMutation.mutate({
                  to: testTo,
                  profile: toProto(provider, form.getFieldsValue()),
                })
              }}
            >
              {m.systemPage_sendTestEmail()}
            </Button>
          </div>
        </Form>
      )}
    </ProfileFormDrawer>
  )
}
