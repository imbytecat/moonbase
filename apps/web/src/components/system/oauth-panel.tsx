import { ApiOutlined, WechatOutlined } from '@ant-design/icons'
import { useMutation } from '@connectrpc/connect-query'
import {
  bindOauthPurpose,
  deleteOauthProfile,
  type OauthSettings,
  type Profile,
} from '@moonbase/api-client'
import { App } from 'antd'
import { ProfileManager, ProviderTag } from '#components/profile-manager'
import { OauthProfileDrawer } from '#components/system/oauth-profile-drawer'
import { humanizeError } from '#lib/errors'
import { useEditingTarget } from '#lib/use-editing-target'

const PURPOSE_LABELS: Record<string, () => string> = {
  login: () => '登录页',
}

const PROVIDER_NAMES: Record<string, () => string> = {
  oidc: () => 'OIDC',
  wechat: () => '微信扫码',
}

export function OauthPanel({
  oauth,
  onChanged,
}: {
  oauth: OauthSettings | undefined
  onChanged: () => void
}) {
  const { message } = App.useApp()
  const profiles = oauth?.profiles ?? []
  const bindings = oauth?.bindings ?? []
  const drawer = useEditingTarget<Profile>()

  const deleteMutation = useMutation(deleteOauthProfile, {
    onSuccess: () => {
      onChanged()
      message.success('存储配置已删除')
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const bindMutation = useMutation(bindOauthPurpose, {
    onSuccess: () => {
      onChanged()
      message.success('设置已保存')
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  return (
    <>
      <ProfileManager
        profiles={profiles}
        bindings={bindings.map((b) => ({
          purpose: b.purpose,
          profileIds: b.profileIds,
          multiple: true,
        }))}
        texts={{
          profilesTitle: '登录配置',
          profilesHint: '可添加多个第三方登录配置，例如企业 SSO 和微信扫码',
          noProfiles: '尚未添加登录配置',
          confirmDelete: '删除该存储配置？',
          bindingsHint: '为登录页指定启用的登录配置，可多选；未绑定的配置将不显示',
        }}
        purposeLabel={(purpose) => PURPOSE_LABELS[purpose]?.() ?? purpose}
        profileIcon={(p) =>
          p.provider === 'wechat' ? (
            <WechatOutlined className="text-lg text-(--ant-color-success)" />
          ) : (
            <ApiOutlined className="text-lg text-(--ant-color-primary)" />
          )
        }
        profileTags={(p) => <ProviderTag name={PROVIDER_NAMES[p.provider]?.() ?? p.provider} />}
        profileDescription={(p) =>
          p.provider === 'wechat' ? String(p.config?.appId ?? '') : String(p.config?.issuer ?? '')
        }
        onAdd={drawer.add}
        onEdit={drawer.edit}
        onDelete={(p) => deleteMutation.mutate({ id: p.id })}
        deleting={deleteMutation.isPending}
        onBind={(purpose, profileIds) => bindMutation.mutate({ purpose, profileIds })}
        binding={bindMutation.isPending}
      />

      <OauthProfileDrawer
        key={drawer.drawerKey}
        profile={drawer.profile}
        open={drawer.open}
        onClose={drawer.close}
        onChanged={() => {
          drawer.close()
          onChanged()
        }}
      />
    </>
  )
}
