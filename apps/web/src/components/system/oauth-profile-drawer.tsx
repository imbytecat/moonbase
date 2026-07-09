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
      message.success('存储配置已创建')
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const updateMutation = useMutation(updateOauthProfile, {
    onSuccess: () => {
      message.success('设置已保存')
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const providers: ProviderOption[] = [
    {
      value: 'oidc',
      label: '通用 OIDC',
      description: '通用 OpenID Connect，覆盖 Google、Keycloak、Authentik 等',
      icon: <ApiOutlined className="text-xl text-(--ant-color-primary)" />,
    },
    {
      value: 'wechat',
      label: '微信扫码',
      description: '微信开放平台网站应用扫码登录',
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
                label={'显示名称'}
                extra={'登录按钮上显示的文字'}
                rules={[{ required: true, message: '请输入配置名称' }]}
              >
                <Input placeholder="Google" />
              </Form.Item>
              {fields.map((field) => (
                <SchemaField key={field.key} field={field} profile={profile} />
              ))}
            </div>
            {!isNew ? (
              <Typography.Paragraph type="secondary">{'创建后不可修改'}</Typography.Paragraph>
            ) : null}

            <Form.Item label={'回调地址'} extra={'在身份服务的应用设置中登记此地址'}>
              <Typography.Text code copyable>
                {callbackUrl}
              </Typography.Text>
            </Form.Item>

            <Button
              type="primary"
              htmlType="submit"
              loading={createMutation.isPending || updateMutation.isPending}
            >
              {'保存'}
            </Button>
          </Form>
        )
      }}
    </ProfileFormDrawer>
  )
}
