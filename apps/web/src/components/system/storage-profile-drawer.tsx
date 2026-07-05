import { CloudServerOutlined, HddOutlined } from '@ant-design/icons'
import { useMutation } from '@connectrpc/connect-query'
import {
  createStorageProfile,
  type StorageProfile,
  testStorageConnection,
  updateStorageProfile,
} from '@moonbase/api-client'
import { App, Button, Form, Input, Switch } from 'antd'
import { useState } from 'react'
import { ProfileFormDrawer, type ProviderOption } from '#components/profile-form-drawer'
import { TestAlert, type TestState } from '#components/system/test-alert'
import { humanizeError } from '#lib/errors'
import { m } from '#paraglide/messages.js'

interface StorageProfileFormValues {
  name: string
  local: { directory: string }
  s3: {
    endpoint: string
    region: string
    bucket: string
    accessKeyId: string
    secretAccessKey: string
    useSsl: boolean
    publicBaseUrl: string
  }
}

export function StorageProfileDrawer({
  profile,
  open,
  onClose,
  onChanged,
}: {
  profile: StorageProfile | undefined
  open: boolean
  onClose: () => void
  onChanged: () => void
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<StorageProfileFormValues>()
  const [result, setResult] = useState<TestState>()

  const createMutation = useMutation(createStorageProfile, {
    onSuccess: () => {
      message.success(m.systemPage_profileCreated())
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const updateMutation = useMutation(updateStorageProfile, {
    onSuccess: () => {
      message.success(m.systemPage_saved())
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
      label: m.systemPage_storageLocal(),
      description: m.systemPage_storageLocalDesc(),
      icon: <HddOutlined className="text-xl text-(--ant-color-primary)" />,
    },
    {
      value: 's3',
      label: m.systemPage_storageS3(),
      description: m.systemPage_storageS3Desc(),
      icon: <CloudServerOutlined className="text-xl text-(--ant-color-warning)" />,
    },
  ]

  const storedLocal = () => ({ directory: profile?.local?.directory ?? '' })
  const storedS3 = () => ({
    endpoint: profile?.s3?.endpoint ?? '',
    region: profile?.s3?.region ?? '',
    bucket: profile?.s3?.bucket ?? '',
    accessKeyId: profile?.s3?.accessKeyId ?? '',
    secretAccessKey: '',
    useSsl: profile?.s3?.useSsl ?? true,
    publicBaseUrl: profile?.s3?.publicBaseUrl ?? '',
  })

  const toProto = (provider: string, values: StorageProfileFormValues) => ({
    id: profile?.id ?? '',
    name: values.name ?? '',
    provider,
    local: provider === 'local' ? values.local : storedLocal(),
    s3: provider === 's3' ? values.s3 : storedS3(),
  })

  const secretPlaceholder = profile?.s3?.secretAccessKeySet ? m.systemPage_secretUnchanged() : ''

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
            local: storedLocal(),
            s3: storedS3(),
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
            <Input placeholder={m.systemPage_profileNamePlaceholder()} />
          </Form.Item>

          {provider === 'local' ? (
            <Form.Item
              name={['local', 'directory']}
              label={m.systemPage_storageDirectory()}
              extra={m.systemPage_storageDirectoryHint()}
            >
              <Input placeholder="data/storage" />
            </Form.Item>
          ) : (
            <>
              <Form.Item name={['s3', 'endpoint']} label={m.systemPage_endpoint()}>
                <Input placeholder="s3.amazonaws.com" />
              </Form.Item>
              <div className="grid grid-cols-2 gap-4">
                <Form.Item name={['s3', 'region']} label={m.systemPage_region()}>
                  <Input placeholder="us-east-1" />
                </Form.Item>
                <Form.Item name={['s3', 'bucket']} label={m.systemPage_bucket()}>
                  <Input placeholder="my-bucket" />
                </Form.Item>
                <Form.Item name={['s3', 'accessKeyId']} label={m.systemPage_accessKeyId()}>
                  <Input autoComplete="off" />
                </Form.Item>
                <Form.Item name={['s3', 'secretAccessKey']} label={m.systemPage_secretAccessKey()}>
                  <Input.Password autoComplete="new-password" placeholder={secretPlaceholder} />
                </Form.Item>
              </div>
              <Form.Item
                name={['s3', 'useSsl']}
                label={m.systemPage_useSsl()}
                valuePropName="checked"
              >
                <Switch />
              </Form.Item>
              <Form.Item
                name={['s3', 'publicBaseUrl']}
                label={m.systemPage_publicBaseUrl()}
                extra={m.systemPage_publicBaseUrlHint()}
              >
                <Input placeholder="https://cdn.example.com" />
              </Form.Item>
            </>
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
            <Button
              loading={testMutation.isPending}
              onClick={() => {
                setResult(undefined)
                testMutation.mutate({ profile: toProto(provider, form.getFieldsValue()) })
              }}
            >
              {m.systemPage_testConnection()}
            </Button>
          </div>
        </Form>
      )}
    </ProfileFormDrawer>
  )
}
