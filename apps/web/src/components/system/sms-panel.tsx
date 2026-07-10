import { MessageOutlined } from '@ant-design/icons'
import { useMutation } from '@connectrpc/connect-query'
import {
  bindSmsPurpose,
  deleteSmsProfile,
  type Profile,
  type SmsSettings,
} from '@moonbase/api-client'
import { App } from 'antd'
import { ProfileManager, ProviderTag } from '#components/profile-manager'
import { SmsProfileDrawer } from '#components/system/sms-profile-drawer'
import { humanizeError } from '#lib/errors'
import { useEditingTarget } from '#lib/use-editing-target'

const PURPOSE_LABELS: Record<string, () => string> = {
  verification: () => '验证码短信',
}

const PROVIDER_NAMES: Record<string, () => string> = {
  aliyun: () => '阿里云短信',
  tencent: () => '腾讯云短信',
}

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

  const signOf = (p: Profile) => String(p.config?.signName ?? '')

  return (
    <>
      <ProfileManager
        profiles={profiles.map((p) => ({ ...p, name: p.name || (signOf(p) ?? '') }))}
        bindings={bindings.map((b) => ({
          purpose: b.purpose,
          profileIds: b.profileId ? [b.profileId] : [],
        }))}
        texts={{
          profilesTitle: '短信配置',
          profilesHint: '可添加多个短信配置，例如国内通道和国际通道',
          noProfiles: '尚未添加短信配置',
          confirmDelete: '删除该存储配置？',
          bindingsHint: '为每类短信指定使用的配置，未绑定的功能将不可用',
        }}
        purposeLabel={(purpose) => PURPOSE_LABELS[purpose]?.() ?? purpose}
        profileIcon={() => <MessageOutlined className="text-lg text-(--ant-color-primary)" />}
        profileTags={(p) => <ProviderTag name={PROVIDER_NAMES[p.provider]?.() ?? p.provider} />}
        profileDescription={(p) => signOf(p) || '签名'}
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
