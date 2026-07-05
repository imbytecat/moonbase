import { ApiOutlined, WechatOutlined } from '@ant-design/icons'
import { useMutation } from '@connectrpc/connect-query'
import { createOauthProfile, type OauthProfile, updateOauthProfile } from '@moonbase/api-client'
import { App, Button, Form, Input, Typography } from 'antd'
import { ProfileFormDrawer, type ProviderOption } from '#components/profile-form-drawer'
import { humanizeError } from '#lib/errors'
import { m } from '#paraglide/messages.js'

interface OauthProfileFormValues {
  key: string
  name: string
  oidc: { issuer: string; clientId: string; clientSecret: string; scopes: string }
  wechat: { appId: string; appSecret: string }
}

export function OauthProfileDrawer({
  profile,
  open,
  onClose,
  onChanged,
}: {
  profile: OauthProfile | undefined
  open: boolean
  onClose: () => void
  onChanged: () => void
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<OauthProfileFormValues>()
  const isNew = !profile
  const watchedKey = Form.useWatch('key', form) ?? (profile?.key || '')

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

  const storedOidc = () => ({
    issuer: profile?.oidc?.issuer ?? '',
    clientId: profile?.oidc?.clientId ?? '',
    clientSecret: '',
    scopes: profile?.oidc?.scopes ?? '',
  })
  const storedWechat = () => ({
    appId: profile?.wechat?.appId ?? '',
    appSecret: '',
  })

  const oidcSecretPlaceholder = profile?.oidc?.clientSecretSet ? m.systemPage_secretUnchanged() : ''
  const wechatSecretPlaceholder = profile?.wechat?.appSecretSet
    ? m.systemPage_secretUnchanged()
    : ''
  const callbackUrl = `${window.location.origin}/api/oauth/${watchedKey || '…'}/callback`

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
            key: profile?.key ?? '',
            name: profile?.name ?? '',
            oidc: storedOidc(),
            wechat: storedWechat(),
          }}
          onFinish={(values) => {
            const p = {
              id: profile?.id ?? '',
              key: values.key ?? '',
              name: values.name ?? '',
              provider,
              oidc: provider === 'oidc' ? values.oidc : storedOidc(),
              wechat: provider === 'wechat' ? values.wechat : storedWechat(),
            }
            if (isNew) createMutation.mutate({ profile: p })
            else updateMutation.mutate({ profile: p })
          }}
        >
          <div className="grid grid-cols-2 gap-4">
            <Form.Item
              name="key"
              label={m.systemPage_oauthKey()}
              extra={isNew ? m.systemPage_oauthKeyHint() : m.systemPage_oauthKeyImmutable()}
              rules={[
                { required: true, message: m.systemPage_oauthKeyRule() },
                { pattern: /^[a-z][a-z0-9-]{1,31}$/, message: m.systemPage_oauthKeyRule() },
              ]}
            >
              <Input placeholder="google" disabled={!isNew} />
            </Form.Item>
            <Form.Item
              name="name"
              label={m.systemPage_oauthDisplayName()}
              extra={m.systemPage_oauthDisplayNameHint()}
              rules={[{ required: true, message: m.systemPage_profileNameRule() }]}
            >
              <Input placeholder="Google" />
            </Form.Item>
          </div>
          {provider === 'oidc' ? (
            <>
              <Form.Item
                name={['oidc', 'issuer']}
                label={m.systemPage_oauthIssuer()}
                extra={m.systemPage_oauthIssuerHint()}
              >
                <Input placeholder="https://accounts.google.com" />
              </Form.Item>
              <div className="grid grid-cols-2 gap-4">
                <Form.Item name={['oidc', 'clientId']} label="Client ID">
                  <Input autoComplete="off" />
                </Form.Item>
                <Form.Item name={['oidc', 'clientSecret']} label="Client Secret">
                  <Input.Password autoComplete="new-password" placeholder={oidcSecretPlaceholder} />
                </Form.Item>
              </div>
              <Form.Item
                name={['oidc', 'scopes']}
                label={m.systemPage_oauthScopes()}
                extra={m.systemPage_oauthScopesHint()}
              >
                <Input placeholder="openid profile email" />
              </Form.Item>
            </>
          ) : (
            <>
              <Typography.Paragraph type="secondary">
                {m.systemPage_oauthWechatHint()}
              </Typography.Paragraph>
              <div className="grid grid-cols-2 gap-4">
                <Form.Item name={['wechat', 'appId']} label="AppID">
                  <Input autoComplete="off" />
                </Form.Item>
                <Form.Item name={['wechat', 'appSecret']} label="AppSecret">
                  <Input.Password
                    autoComplete="new-password"
                    placeholder={wechatSecretPlaceholder}
                  />
                </Form.Item>
              </div>
            </>
          )}

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
      )}
    </ProfileFormDrawer>
  )
}
