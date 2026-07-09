import { ApiOutlined, WechatOutlined } from '@ant-design/icons'
import { useMutation, useQuery } from '@connectrpc/connect-query'
import {
  createOauthProfile,
  describeOauthProviders,
  type Profile,
  updateOauthProfile,
} from '@moonbase/api-client'
import { App, Button, Form, Input, Typography } from 'antd'
import { ProfileFormDrawer, type ProviderOption } from '#components/profile-form-drawer'
import {
  SchemaField,
  type SchemaProfileFormValues,
  schemaInitialConfig,
  schemaProfileToProto,
} from '#components/system/schema-profile-form'
import { humanizeError } from '#lib/errors'
import { m } from '#paraglide/messages.js'

export function OauthProfileDrawer({
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
  const isNew = !profile
  const watchedKey = Form.useWatch(['config', 'key'], form) ?? String(profile?.config?.key || '')

  const { data: describe } = useQuery(describeOauthProviders, {})
  const schemas = describe?.providers ?? {}

  const createMutation = useMutation(createOauthProfile, {
    onSuccess: () => {
      message.success(m.systemPage_profileCreated())
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const updateMutation = useMutation(updateOauthProfile, {
    onSuccess: () => {
      message.success(m.systemPage_saved())
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const providers: ProviderOption[] = [
    {
      value: 'oidc',
      label: m.systemPage_oauthOidc(),
      description: m.systemPage_oauthOidcDesc(),
      icon: <ApiOutlined className="text-xl text-(--ant-color-primary)" />,
    },
    {
      value: 'wechat',
      label: m.systemPage_oauthWechat(),
      description: m.systemPage_oauthWechatDesc(),
      icon: <WechatOutlined className="text-xl text-(--ant-color-success)" />,
    },
  ]

  const toProto = (provider: string, values: SchemaProfileFormValues) =>
    schemaProfileToProto(profile, provider, schemas[provider]?.fields ?? [], values)

  const callbackUrl = `${window.location.origin}/api/oauth/${watchedKey || '…'}/callback`

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
              if (isNew) createMutation.mutate({ profile: p })
              else updateMutation.mutate({ profile: p })
            }}
          >
            <div className="grid grid-cols-2 gap-4">
              <Form.Item
                name="name"
                label={m.systemPage_oauthDisplayName()}
                extra={m.systemPage_oauthDisplayNameHint()}
                rules={[{ required: true, message: m.systemPage_profileNameRule() }]}
              >
                <Input placeholder="Google" />
              </Form.Item>
              {fields.map((field) => (
                <SchemaField key={field.key} field={field} profile={profile} />
              ))}
            </div>
            {!isNew ? (
              <Typography.Paragraph type="secondary">
                {m.systemPage_oauthKeyImmutable()}
              </Typography.Paragraph>
            ) : null}

            <Form.Item
              label={m.systemPage_oauthCallbackUrl()}
              extra={m.systemPage_oauthCallbackHint()}
            >
              <Typography.Text code copyable>
                {callbackUrl}
              </Typography.Text>
            </Form.Item>

            <Button
              type="primary"
              htmlType="submit"
              loading={createMutation.isPending || updateMutation.isPending}
            >
              {m.common_save()}
            </Button>
          </Form>
        )
      }}
    </ProfileFormDrawer>
  )
}
