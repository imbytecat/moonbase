import { ApiOutlined, WechatOutlined } from '@ant-design/icons'
import { useMutation } from '@connectrpc/connect-query'
import {
  bindOauthPurpose,
  deleteOauthProfile,
  type OauthSettings,
  type Profile,
} from '@moonbase/api-client'
import { App } from 'antd'
import { useState } from 'react'
import { ProfileManager, ProviderTag } from '#components/profile-manager'
import { OauthProfileDrawer } from '#components/system/oauth-profile-drawer'
import { humanizeError } from '#lib/errors'
import { m } from '#paraglide/messages.js'

const PURPOSE_LABELS: Record<string, () => string> = {
  login: m.systemPage_oauthPurposeLogin,
}

const PROVIDER_NAMES: Record<string, () => string> = {
  oidc: () => 'OIDC',
  wechat: m.systemPage_oauthWechat,
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
  const [editing, setEditing] = useState<Profile | 'new' | undefined>()

  const deleteMutation = useMutation(deleteOauthProfile, {
    onSuccess: () => {
      onChanged()
      message.success(m.systemPage_profileDeleted())
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const bindMutation = useMutation(bindOauthPurpose, {
    onSuccess: () => {
      onChanged()
      message.success(m.systemPage_saved())
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
          profilesTitle: m.systemPage_oauthProfilesTitle(),
          profilesHint: m.systemPage_oauthProfilesHint(),
          noProfiles: m.systemPage_oauthNoProfiles(),
          confirmDelete: m.systemPage_confirmDeleteProfile(),
          bindingsHint: m.systemPage_oauthBindingsHint(),
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
        onAdd={() => setEditing('new')}
        onEdit={(p) => setEditing(p)}
        onDelete={(p) => deleteMutation.mutate({ id: p.id })}
        deleting={deleteMutation.isPending}
        onBind={(purpose, profileIds) => bindMutation.mutate({ purpose, profileIds })}
        binding={bindMutation.isPending}
      />

      <OauthProfileDrawer
        key={editing === 'new' ? 'new' : (editing?.id ?? 'closed')}
        profile={editing === 'new' ? undefined : editing}
        open={editing !== undefined}
        onClose={() => setEditing(undefined)}
        onChanged={() => {
          setEditing(undefined)
          onChanged()
        }}
      />
    </>
  )
}
