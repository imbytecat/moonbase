import { MailOutlined } from '@ant-design/icons'
import { useMutation } from '@connectrpc/connect-query'
import {
  bindEmailPurpose,
  deleteEmailProfile,
  type EmailSettings,
  type Profile,
} from '@moonbase/api-client'
import { App } from 'antd'
import { useState } from 'react'
import { ProfileManager, ProviderTag } from '#components/profile-manager'
import { EmailProfileDrawer } from '#components/system/email-profile-drawer'
import { humanizeError } from '#lib/errors'

const PURPOSE_LABELS: Record<string, () => string> = {
  auth: () => '账号验证邮件',
}

const PROVIDER_NAMES: Record<string, string> = {
  smtp: 'SMTP',
  cloudflare: 'Cloudflare',
}

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
  const [editing, setEditing] = useState<Profile | 'new' | undefined>()

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
        profiles={profiles.map((p) => ({
          ...p,
          name: p.name || String(p.config?.fromAddress ?? ''),
        }))}
        bindings={bindings.map((b) => ({
          purpose: b.purpose,
          profileIds: b.profileId ? [b.profileId] : [],
        }))}
        texts={{
          profilesTitle: '邮件配置',
          profilesHint: '可添加多个发信配置，例如验证码专用通道和通知通道',
          noProfiles: '尚未添加邮件配置',
          confirmDelete: '删除该存储配置？',
          bindingsHint: '为每类邮件指定使用的发信配置，未绑定的功能将不可用',
        }}
        purposeLabel={(purpose) => PURPOSE_LABELS[purpose]?.() ?? purpose}
        profileIcon={() => <MailOutlined className="text-lg text-(--ant-color-primary)" />}
        profileTags={(p) => <ProviderTag name={PROVIDER_NAMES[p.provider] ?? p.provider} />}
        profileDescription={(p) => String(p.config?.fromAddress || '发件地址')}
        onAdd={() => setEditing('new')}
        onEdit={(p) => setEditing(p)}
        onDelete={(p) => deleteMutation.mutate({ id: p.id })}
        deleting={deleteMutation.isPending}
        onBind={(purpose, ids) => bindMutation.mutate({ purpose, profileId: ids[0] ?? '' })}
        binding={bindMutation.isPending}
      />

      <EmailProfileDrawer
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
