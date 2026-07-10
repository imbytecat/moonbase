import { useMutation, useQuery } from '@connectrpc/connect-query'
import {
  bindCaptchaPurpose,
  type CaptchaSettings,
  createCaptchaProfile,
  deleteCaptchaProfile,
  describeCaptchaProviders,
  type Profile,
  updateCaptchaProfile,
} from '@moonbase/api-client'
import { App } from 'antd'
import { useState } from 'react'
import { ProfileFormDrawer } from '#components/profile-form-drawer'
import { ProfileManager } from '#components/profile-manager'
import { ConfigForm } from '#components/system/config-form'
import { humanizeError } from '#lib/errors'
import { useEditingTarget } from '#lib/use-editing-target'

export function CaptchaPanel({
  captcha,
  onChanged,
}: {
  captcha: CaptchaSettings | undefined
  onChanged: () => void
}) {
  const { message } = App.useApp()
  const profiles = captcha?.profiles ?? []
  const bindings = captcha?.bindings ?? []
  const drawer = useEditingTarget<Profile>()
  const { data: describe } = useQuery(describeCaptchaProviders, {})

  const deleteMutation = useMutation(deleteCaptchaProfile, {
    onSuccess: () => {
      onChanged()
      message.success('存储配置已删除')
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const bindMutation = useMutation(bindCaptchaPurpose, {
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
          profileIds: b.profileId ? [b.profileId] : [],
        }))}
        purposes={describe?.purposes ?? []}
        providers={describe?.providers ?? []}
        texts={{
          profilesTitle: '验证配置',
          profilesHint: '可添加多个人机验证配置，按用途选择启用',
          noProfiles: '尚未添加人机验证配置',
          confirmDelete: '删除该存储配置？',
          bindingsHint: '为每个场景指定使用的验证配置，未绑定的场景不启用人机验证',
        }}
        onAdd={drawer.add}
        onEdit={drawer.edit}
        onDelete={(p) => deleteMutation.mutate({ id: p.id })}
        deleting={deleteMutation.isPending}
        onBind={(purpose, ids) => bindMutation.mutate({ purpose, profileId: ids[0] ?? '' })}
        binding={bindMutation.isPending}
      />

      <CaptchaProfileDrawer
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

function CaptchaProfileDrawer({
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

  const { data: describe } = useQuery(describeCaptchaProviders, {})
  const providers = describe?.providers ?? []

  const createMutation = useMutation(createCaptchaProfile, {
    onSuccess: () => {
      message.success('存储配置已创建')
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const updateMutation = useMutation(updateCaptchaProfile, {
    onSuccess: () => {
      message.success('设置已保存')
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  return (
    <ProfileFormDrawer
      open={open}
      onClose={onClose}
      dirty={dirty}
      profileProvider={profile?.provider}
      providers={providers}
    >
      {(provider) => {
        const providerForm = providers.find((item) => item.key === provider)?.config
        if (!providerForm) return null
        return (
          <ConfigForm
            key={provider}
            providerForm={providerForm}
            provider={provider}
            profile={profile}
            saving={createMutation.isPending || updateMutation.isPending}
            onDirtyChange={setDirty}
            onSubmit={(p) => {
              if (profile) updateMutation.mutate({ profile: p })
              else createMutation.mutate({ profile: p })
            }}
          />
        )
      }}
    </ProfileFormDrawer>
  )
}
