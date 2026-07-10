import { useMutation, useQuery } from '@connectrpc/connect-query'
import {
  bindEmailPurpose,
  deleteEmailProfile,
  describeEmailProviders,
  type EmailSettings,
  type Profile,
} from '@moonbase/api-client'
import { App } from 'antd'
import { ProfileManager } from '#components/profile-manager'
import { EmailProfileDrawer } from '#components/system/email-profile-drawer'
import { humanizeError } from '#lib/errors'
import { useEditingTarget } from '#lib/use-editing-target'

export function EmailPanel({
  email,
  onChanged,
}: {
  email: EmailSettings | undefined
  onChanged: () => void
}) {
  const { message } = App.useApp()
  const profiles = email?.profiles ?? []
  const bindings = email?.bindings ?? []
  const drawer = useEditingTarget<Profile>()
  const { data: describe } = useQuery(describeEmailProviders, {})

  const deleteMutation = useMutation(deleteEmailProfile, {
    onSuccess: () => {
      onChanged()
      message.success('存储配置已删除')
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const bindMutation = useMutation(bindEmailPurpose, {
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
          profilesTitle: '邮件配置',
          profilesHint: '可添加多个发信配置，例如验证码专用通道和通知通道',
          noProfiles: '尚未添加邮件配置',
          confirmDelete: '删除该存储配置？',
          bindingsHint: '为每类邮件指定使用的发信配置，未绑定的功能将不可用',
        }}
        onAdd={drawer.add}
        onEdit={drawer.edit}
        onDelete={(p) => deleteMutation.mutate({ id: p.id })}
        deleting={deleteMutation.isPending}
        onBind={(purpose, ids) => bindMutation.mutate({ purpose, profileId: ids[0] ?? '' })}
        binding={bindMutation.isPending}
      />

      <EmailProfileDrawer
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
