import { MailOutlined } from '@ant-design/icons'
import { useMutation } from '@connectrpc/connect-query'
import {
  bindEmailPurpose,
  deleteEmailProfile,
  type EmailProfile,
  type EmailSettings,
} from '@moonbase/api-client'
import { App } from 'antd'
import { useState } from 'react'
import { ProfileManager, ProviderTag } from '#components/profile-manager'
import { EmailProfileDrawer } from '#components/system/email-profile-drawer'
import { humanizeError } from '#lib/errors'
import { m } from '#paraglide/messages.js'

const PURPOSE_LABELS: Record<string, () => string> = {
  auth: m.systemPage_emailPurposeAuth,
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
  const [editing, setEditing] = useState<EmailProfile | 'new' | undefined>()

  const deleteMutation = useMutation(deleteEmailProfile, {
    onSuccess: () => {
      onChanged()
      message.success(m.systemPage_profileDeleted())
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const bindMutation = useMutation(bindEmailPurpose, {
    onSuccess: () => {
      onChanged()
      message.success(m.systemPage_saved())
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  return (
    <>
      <ProfileManager
        profiles={profiles.map((p) => ({ ...p, name: p.name || p.fromAddress }))}
        bindings={bindings.map((b) => ({
          purpose: b.purpose,
          profileIds: b.profileId ? [b.profileId] : [],
        }))}
        texts={{
          profilesTitle: m.systemPage_emailProfilesTitle(),
          profilesHint: m.systemPage_emailProfilesHint(),
          noProfiles: m.systemPage_emailNoProfiles(),
          confirmDelete: m.systemPage_confirmDeleteProfile(),
          bindingsHint: m.systemPage_emailBindingsHint(),
        }}
        purposeLabel={(purpose) => PURPOSE_LABELS[purpose]?.() ?? purpose}
        profileIcon={() => <MailOutlined className="text-lg text-(--ant-color-primary)" />}
        profileTags={(p) => <ProviderTag name={PROVIDER_NAMES[p.provider] ?? p.provider} />}
        profileDescription={(p) => p.fromAddress || m.systemPage_fromAddress()}
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
