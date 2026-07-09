import { AlipayCircleOutlined, WechatOutlined } from '@ant-design/icons'
import { useMutation, useQuery } from '@connectrpc/connect-query'
import {
  createPaymentProfile,
  describePaymentProviders,
  type Profile,
  updatePaymentProfile,
} from '@moonbase/api-client'
import { App, Button, Form, Input } from 'antd'
import { ProfileFormDrawer, type ProviderOption } from '#components/profile-form-drawer'
import {
  SchemaField,
  type SchemaProfileFormValues,
  schemaInitialConfig,
  schemaProfileToProto,
} from '#components/system/schema-profile-form'
import { humanizeError } from '#lib/errors'
import { m } from '#paraglide/messages.js'

export function PaymentProfileDrawer({
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

  const { data: describe } = useQuery(describePaymentProviders, {})
  const schemas = describe?.providers ?? {}

  const createMutation = useMutation(createPaymentProfile, {
    onSuccess: () => {
      message.success(m.systemPage_profileCreated())
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const updateMutation = useMutation(updatePaymentProfile, {
    onSuccess: () => {
      message.success(m.systemPage_saved())
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const providers: ProviderOption[] = [
    {
      value: 'alipay',
      label: m.systemPage_providerAlipay(),
      description: m.systemPage_alipayDesc(),
      icon: <AlipayCircleOutlined className="text-xl text-(--ant-color-info)" />,
    },
    {
      value: 'wechat',
      label: m.systemPage_providerWechatPay(),
      description: m.systemPage_wechatPayDesc(),
      icon: <WechatOutlined className="text-xl text-(--ant-color-success)" />,
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
              <Input placeholder={m.systemPage_paymentProfileNamePlaceholder()} />
            </Form.Item>

            <div className="grid grid-cols-2 gap-4">
              {fields.map((field) => (
                <SchemaField key={field.key} field={field} profile={profile} />
              ))}
            </div>

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
