import { MessageOutlined } from '@ant-design/icons'
import { useMutation } from '@connectrpc/connect-query'
import {
  bindSmsPurpose,
  deleteSmsProfile,
  type SmsProfile,
  type SmsSettings,
} from '@moonbase/api-client'
import { App } from 'antd'
import { useState } from 'react'
import { ProfileManager, ProviderTag } from '#components/profile-manager'
import { SmsProfileDrawer } from '#components/system/sms-profile-drawer'
import { humanizeError } from '#lib/errors'
import { m } from '#paraglide/messages.js'

const PURPOSE_LABELS: Record<string, () => string> = {
  verification: m.systemPage_smsPurposeVerification,
}

const PROVIDER_NAMES: Record<string, () => string> = {
  aliyun: m.systemPage_providerAliyun,
  tencent: m.systemPage_providerTencent,
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
  const [editing, setEditing] = useState<SmsProfile | 'new' | undefined>()

  const deleteMutation = useMutation(deleteSmsProfile, {
    onSuccess: () => {
      onChanged()
      message.success(m.systemPage_profileDeleted())
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const bindMutation = useMutation(bindSmsPurpose, {
    onSuccess: () => {
      onChanged()
      message.success(m.systemPage_saved())
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const signOf = (p: SmsProfile) =>
    p.provider === 'tencent' ? p.tencent?.signName : p.aliyun?.signName

  return (
    <>
      <ProfileManager
        profiles={profiles.map((p) => ({ ...p, name: p.name || (signOf(p) ?? '') }))}
        bindings={bindings.map((b) => ({
          purpose: b.purpose,
          profileIds: b.profileId ? [b.profileId] : [],
        }))}
        texts={{
          profilesTitle: m.systemPage_smsProfilesTitle(),
          profilesHint: m.systemPage_smsProfilesHint(),
          noProfiles: m.systemPage_smsNoProfiles(),
          confirmDelete: m.systemPage_confirmDeleteProfile(),
          bindingsHint: m.systemPage_smsBindingsHint(),
        }}
        purposeLabel={(purpose) => PURPOSE_LABELS[purpose]?.() ?? purpose}
        profileIcon={() => <MessageOutlined className="text-lg text-(--ant-color-primary)" />}
        profileTags={(p) => <ProviderTag name={PROVIDER_NAMES[p.provider]?.() ?? p.provider} />}
        profileDescription={(p) => signOf(p) || m.systemPage_signName()}
        onAdd={() => setEditing('new')}
        onEdit={(p) => setEditing(p)}
        onDelete={(p) => deleteMutation.mutate({ id: p.id })}
        deleting={deleteMutation.isPending}
        onBind={(purpose, ids) => bindMutation.mutate({ purpose, profileId: ids[0] ?? '' })}
        binding={bindMutation.isPending}
      />

      <SmsProfileDrawer
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
