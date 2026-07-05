import { AlipayCircleOutlined, WechatOutlined } from '@ant-design/icons'
import { useMutation } from '@connectrpc/connect-query'
import {
  createPaymentProfile,
  type PaymentProfile,
  updatePaymentProfile,
} from '@moonbase/api-client'
import { App, Button, Checkbox, Form, Input, Segmented } from 'antd'
import { useState } from 'react'
import { ProfileFormDrawer, type ProviderOption } from '#components/profile-form-drawer'
import { humanizeError } from '#lib/errors'
import { methodDesc, methodLabel, PROVIDER_METHODS } from '#lib/payments'
import { m } from '#paraglide/messages.js'

interface PaymentProfileFormValues {
  name: string
  alipay: {
    appId: string
    appPrivateKey: string
    authMethod: string
    alipayPublicKey: string
    appCert: string
    alipayRootCert: string
    alipayPublicCert: string
    opAppId: string
  }
  wechat: {
    mchId: string
    appId: string
    mchCertSerialNo: string
    mchPrivateKey: string
    apiV3Key: string
    authMethod: string
    publicKeyId: string
    publicKey: string
  }
}

export function PaymentProfileDrawer({
  profile,
  open,
  onClose,
  onChanged,
}: {
  profile: PaymentProfile | undefined
  open: boolean
  onClose: () => void
  onChanged: () => void
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<PaymentProfileFormValues>()
  const [alipayAuth, setAlipayAuth] = useState(profile?.alipay?.authMethod || 'public_key')
  const [wechatAuth, setWechatAuth] = useState(profile?.wechat?.authMethod || 'public_key')
  // null = untouched: default to all of the provider's products (matches the
  // "empty methods = all" back-compat rule the server applies).
  const [methods, setMethods] = useState<string[] | null>(
    profile?.methods?.length ? [...profile.methods] : null,
  )

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

  const storedAlipay = () => ({
    appId: profile?.alipay?.appId ?? '',
    appPrivateKey: '',
    authMethod: profile?.alipay?.authMethod || 'public_key',
    alipayPublicKey: profile?.alipay?.alipayPublicKey ?? '',
    appCert: profile?.alipay?.appCert ?? '',
    alipayRootCert: profile?.alipay?.alipayRootCert ?? '',
    alipayPublicCert: profile?.alipay?.alipayPublicCert ?? '',
    opAppId: profile?.alipay?.opAppId ?? '',
  })
  const storedWechat = () => ({
    mchId: profile?.wechat?.mchId ?? '',
    appId: profile?.wechat?.appId ?? '',
    mchCertSerialNo: profile?.wechat?.mchCertSerialNo ?? '',
    mchPrivateKey: '',
    apiV3Key: '',
    authMethod: profile?.wechat?.authMethod || 'public_key',
    publicKeyId: profile?.wechat?.publicKeyId ?? '',
    publicKey: profile?.wechat?.publicKey ?? '',
  })

  const selectedFor = (provider: string) => {
    const pm = PROVIDER_METHODS[provider] ?? []
    return (methods ?? pm).filter((id) => pm.includes(id))
  }

  const toProto = (provider: string, values: PaymentProfileFormValues) => ({
    id: profile?.id ?? '',
    name: values.name ?? '',
    provider,
    methods: selectedFor(provider),
    alipay: provider === 'alipay' ? { ...values.alipay, authMethod: alipayAuth } : storedAlipay(),
    wechat: provider === 'wechat' ? { ...values.wechat, authMethod: wechatAuth } : storedWechat(),
  })

  const secretPlaceholder = (set: boolean | undefined) =>
    set ? m.systemPage_secretUnchanged() : ''

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
            alipay: storedAlipay(),
            wechat: storedWechat(),
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

          <Form.Item
            label={m.systemPage_paymentMethods()}
            extra={m.systemPage_paymentMethodsHint()}
          >
            <Checkbox.Group
              className="w-full"
              value={selectedFor(provider)}
              onChange={(v) => setMethods(v as string[])}
            >
              <div className="flex flex-col gap-2">
                {(PROVIDER_METHODS[provider] ?? []).map((id) => (
                  <Checkbox key={id} value={id}>
                    {methodLabel(id)}
                    <div className="text-xs text-(--ant-color-text-tertiary)">{methodDesc(id)}</div>
                  </Checkbox>
                ))}
              </div>
            </Checkbox.Group>
          </Form.Item>

          {provider === 'alipay' ? (
            <>
              <Form.Item name={['alipay', 'appId']} label="App ID">
                <Input autoComplete="off" />
              </Form.Item>
              <Form.Item
                name={['alipay', 'appPrivateKey']}
                label={m.systemPage_alipayAppPrivateKey()}
                extra={m.systemPage_alipayAppPrivateKeyHint()}
              >
                <Input.TextArea
                  rows={3}
                  placeholder={secretPlaceholder(profile?.alipay?.appPrivateKeySet)}
                />
              </Form.Item>
              <Form.Item label={m.systemPage_paymentAuthMethod()}>
                <Segmented
                  value={alipayAuth}
                  onChange={(v) => setAlipayAuth(v as string)}
                  options={[
                    { value: 'public_key', label: m.systemPage_paymentAuthPublicKey() },
                    { value: 'cert', label: m.systemPage_paymentAuthCert() },
                  ]}
                />
              </Form.Item>
              {alipayAuth === 'cert' ? (
                <>
                  <Form.Item name={['alipay', 'appCert']} label={m.systemPage_alipayAppCert()}>
                    <Input.TextArea rows={2} />
                  </Form.Item>
                  <Form.Item
                    name={['alipay', 'alipayRootCert']}
                    label={m.systemPage_alipayRootCert()}
                  >
                    <Input.TextArea rows={2} />
                  </Form.Item>
                  <Form.Item
                    name={['alipay', 'alipayPublicCert']}
                    label={m.systemPage_alipayPublicCert()}
                  >
                    <Input.TextArea rows={2} />
                  </Form.Item>
                </>
              ) : (
                <Form.Item
                  name={['alipay', 'alipayPublicKey']}
                  label={m.systemPage_alipayPublicKey()}
                  extra={m.systemPage_alipayPublicKeyHint()}
                >
                  <Input.TextArea rows={3} />
                </Form.Item>
              )}
              {selectedFor('alipay').includes('create') ? (
                <Form.Item
                  name={['alipay', 'opAppId']}
                  label={m.systemPage_alipayOpAppId()}
                  extra={m.systemPage_alipayOpAppIdHint()}
                >
                  <Input autoComplete="off" />
                </Form.Item>
              ) : null}
            </>
          ) : (
            <>
              <div className="grid grid-cols-2 gap-4">
                <Form.Item name={['wechat', 'mchId']} label={m.systemPage_wechatMchId()}>
                  <Input autoComplete="off" />
                </Form.Item>
                <Form.Item name={['wechat', 'appId']} label="App ID">
                  <Input autoComplete="off" />
                </Form.Item>
              </div>
              <Form.Item
                name={['wechat', 'mchCertSerialNo']}
                label={m.systemPage_wechatMchCertSerialNo()}
              >
                <Input autoComplete="off" />
              </Form.Item>
              <Form.Item
                name={['wechat', 'mchPrivateKey']}
                label={m.systemPage_wechatMchPrivateKey()}
                extra={m.systemPage_wechatMchPrivateKeyHint()}
              >
                <Input.TextArea
                  rows={3}
                  placeholder={secretPlaceholder(profile?.wechat?.mchPrivateKeySet)}
                />
              </Form.Item>
              <Form.Item name={['wechat', 'apiV3Key']} label={m.systemPage_wechatApiV3Key()}>
                <Input.Password
                  autoComplete="new-password"
                  placeholder={secretPlaceholder(profile?.wechat?.apiV3KeySet)}
                />
              </Form.Item>
              <Form.Item
                label={m.systemPage_paymentAuthMethod()}
                extra={m.systemPage_wechatAuthMethodHint()}
              >
                <Segmented
                  value={wechatAuth}
                  onChange={(v) => setWechatAuth(v as string)}
                  options={[
                    { value: 'public_key', label: m.systemPage_paymentAuthPublicKey() },
                    { value: 'platform_cert', label: m.systemPage_paymentAuthPlatformCert() },
                  ]}
                />
              </Form.Item>
              {wechatAuth === 'public_key' ? (
                <>
                  <Form.Item
                    name={['wechat', 'publicKeyId']}
                    label={m.systemPage_wechatPublicKeyId()}
                  >
                    <Input autoComplete="off" placeholder="PUB_KEY_ID_..." />
                  </Form.Item>
                  <Form.Item name={['wechat', 'publicKey']} label={m.systemPage_wechatPublicKey()}>
                    <Input.TextArea rows={3} />
                  </Form.Item>
                </>
              ) : null}
            </>
          )}

          <Button
            type="primary"
            htmlType="submit"
            loading={createMutation.isPending || updateMutation.isPending}
          >
            {m.common_save()}
          </Button>
        </Form>
      )}
    </ProfileFormDrawer>
  )
}
