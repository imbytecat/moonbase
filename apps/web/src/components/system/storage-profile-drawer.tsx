import { CloudServerOutlined, HddOutlined } from '@ant-design/icons'
import { useMutation, useQuery } from '@connectrpc/connect-query'
import {
  createStorageProfile,
  describeStorageProviders,
  type Profile,
  testStorageConnection,
  updateStorageProfile,
} from '@moonbase/api-client'
import { App, Button, Form, Input } from 'antd'
import { useState } from 'react'
import { ProfileFormDrawer, type ProviderOption } from '#components/profile-form-drawer'
import {
  SchemaField,
  type SchemaProfileFormValues,
  schemaInitialConfig,
  schemaProfileToProto,
} from '#components/system/schema-profile-form'
import { TestAlert, type TestState } from '#components/system/test-alert'
import { humanizeError } from '#lib/errors'
import { m } from '#paraglide/messages.js'

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
  const [form] = Form.useForm<SchemaProfileFormValues>()
  const [result, setResult] = useState<TestState>()

  const { data: describe } = useQuery(describeStorageProviders, {})
  const schemas = describe?.providers ?? {}

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

  const toProto = (provider: string, values: SchemaProfileFormValues) =>
    schemaProfileToProto(profile, provider, schemas[provider]?.fields ?? [], values)

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
            initialValues={{
              name: profile?.name ?? '',
              config: schemaInitialConfig(profile, provider, fields),
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

            <div className="grid grid-cols-2 gap-4">
              {fields.map((field) => (
                <SchemaField key={field.key} field={field} profile={profile} />
              ))}
            </div>

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
        )
      }}
    </ProfileFormDrawer>
  )
}
