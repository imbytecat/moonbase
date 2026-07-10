import { ApiOutlined, WechatOutlined } from '@ant-design/icons'
import { useMutation, useQuery } from '@connectrpc/connect-query'
import {
  createOauthProfile,
  describeOauthProviders,
  type Profile,
  updateOauthProfile,
} from '@moonbase/api-client'
import { App, Typography } from 'antd'
import { useState } from 'react'
import { ProfileFormDrawer, type ProviderOption } from '#components/profile-form-drawer'
import { ConfigForm } from '#components/system/config-form'
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
  const [dirty, setDirty] = useState(false)

  const { data: describe } = useQuery(describeOauthProviders, {})
  const forms = describe?.providers ?? {}

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

  return (
    <ProfileFormDrawer
      open={open}
      onClose={onClose}
      dirty={dirty}
      profileProvider={profile?.provider}
      providers={providers}
    >
      {(provider) => {
        const providerForm = forms[provider]
        if (!providerForm) return null
        return (
          <ConfigForm
            key={provider}
            providerForm={providerForm}
            provider={provider}
            profile={profile}
            saving={createMutation.isPending || updateMutation.isPending}
            onDirtyChange={setDirty}
            nameField={{ label: '显示名称', placeholder: 'Google', help: '登录按钮上显示的文字' }}
            onSubmit={(p) => {
              if (profile) updateMutation.mutate({ profile: p })
              else createMutation.mutate({ profile: p })
            }}
            banner={(current) => (
              <Typography.Paragraph type="secondary" className="!mb-0">
                {'回调地址（在身份服务的应用设置中登记）：'}
                <Typography.Text code copyable>
                  {`${window.location.origin}/api/oauth/${String(current.config.key || '…')}/callback`}
                </Typography.Text>
              </Typography.Paragraph>
            )}
          />
        )
      }}
    </ProfileFormDrawer>
  )
}
