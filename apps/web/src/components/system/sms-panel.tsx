import { useMutation, useQuery } from '@connectrpc/connect-query'
import {
  bindSmsPurpose,
  deleteSmsProfile,
  describeSmsProviders,
  type Profile,
  type SmsSettings,
} from '@moonbase/api-client'
import { App } from 'antd'
import { ProfileManager } from '#components/profile-manager'
import { SmsProfileDrawer } from '#components/system/sms-profile-drawer'
import { humanizeError } from '#lib/errors'
import { useEditingTarget } from '#lib/use-editing-target'

export function SmsPanel({
  sms,
  onChanged,
}: {
  sms: SmsSettings | undefined
  onChanged: () => void
}) {
  const { message } = App.useApp()
  const profiles = sms?.profiles ?? []
  const bindings = sms?.bindings ?? []
  const drawer = useEditingTarget<Profile>()
  const { data: describe } = useQuery(describeSmsProviders, {})

  const deleteMutation = useMutation(deleteSmsProfile, {
    onSuccess: () => {
      onChanged()
      message.success('存储配置已删除')
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const bindMutation = useMutation(bindSmsPurpose, {
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
          profilesTitle: '短信配置',
          profilesHint: '可添加多个短信配置，例如国内通道和国际通道',
          noProfiles: '尚未添加短信配置',
          confirmDelete: '删除该存储配置？',
          bindingsHint: '为每类短信指定使用的配置，未绑定的功能将不可用',
        }}
        onAdd={drawer.add}
        onEdit={drawer.edit}
        onDelete={(p) => deleteMutation.mutate({ id: p.id })}
        deleting={deleteMutation.isPending}
        onBind={(purpose, ids) => bindMutation.mutate({ purpose, profileId: ids[0] ?? '' })}
        binding={bindMutation.isPending}
      />

      <SmsProfileDrawer
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
