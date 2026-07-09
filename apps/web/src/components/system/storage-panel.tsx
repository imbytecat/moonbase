import { GlobalOutlined, HddOutlined, LockOutlined } from '@ant-design/icons'
import { useMutation } from '@connectrpc/connect-query'
import {
  bindStoragePurpose,
  deleteStorageProfile,
  type Profile,
  type StorageSettings,
} from '@moonbase/api-client'
import { App, Tag } from 'antd'
import { useState } from 'react'
import { ProfileManager, ProviderTag } from '#components/profile-manager'
import { StorageProfileDrawer } from '#components/system/storage-profile-drawer'
import { humanizeError } from '#lib/errors'
import { m } from '#paraglide/messages.js'

const PURPOSE_LABELS: Record<string, () => string> = {
  avatars: m.systemPage_purposeAvatars,
  'site-assets': m.systemPage_purposeSiteAssets,
}

const PROVIDER_NAMES: Record<string, () => string> = {
  local: m.systemPage_storageLocal,
  s3: m.systemPage_storageS3,
}

export function StoragePanel({
  storage,
  onChanged,
}: {
  storage: StorageSettings | undefined
  onChanged: () => void
}) {
  const { message } = App.useApp()
  const profiles = storage?.profiles ?? []
  const bindings = storage?.bindings ?? []
  const [editing, setEditing] = useState<Profile | 'new' | undefined>()

  const deleteMutation = useMutation(deleteStorageProfile, {
    onSuccess: () => {
      onChanged()
      message.success(m.systemPage_profileDeleted())
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const bindMutation = useMutation(bindStoragePurpose, {
    onSuccess: () => {
      onChanged()
      message.success(m.systemPage_saved())
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  return (
    <>
      <ProfileManager
        profiles={profiles.map((p) => ({
          ...p,
          name:
            p.name ||
            (p.provider === 'local' ? m.systemPage_storageLocal() : String(p.config?.bucket ?? '')),
        }))}
        bindings={bindings.map((b) => ({
          purpose: b.purpose,
          profileIds: b.profileId ? [b.profileId] : [],
        }))}
        texts={{
          profilesTitle: m.systemPage_profilesTitle(),
          profilesHint: m.systemPage_profilesHint(),
          noProfiles: m.systemPage_noProfiles(),
          confirmDelete: m.systemPage_confirmDeleteProfile(),
          bindingsHint: m.systemPage_bindingsHint(),
        }}
        purposeLabel={(purpose) => PURPOSE_LABELS[purpose]?.() ?? purpose}
        profileIcon={(p) =>
          p.provider === 'local' ? (
            <HddOutlined className="text-lg text-(--ant-color-primary)" />
          ) : p.config?.publicBaseUrl ? (
            <GlobalOutlined className="text-lg text-(--ant-color-success)" />
          ) : (
            <LockOutlined className="text-lg text-(--ant-color-text-tertiary)" />
          )
        }
        profileTags={(p) => (
          <>
            <ProviderTag name={PROVIDER_NAMES[p.provider]?.() ?? p.provider} />
            {p.provider === 's3' ? (
              <Tag color={p.config?.publicBaseUrl ? 'green' : 'default'}>
                {p.config?.publicBaseUrl
                  ? m.systemPage_publicBucket()
                  : m.systemPage_privateBucket()}
              </Tag>
            ) : null}
          </>
        )}
        profileDescription={(p) =>
          p.provider === 'local'
            ? String(p.config?.directory || m.systemPage_storageDefaultDirectory())
            : `${String(p.config?.endpoint ?? '')} / ${String(p.config?.bucket ?? '')}`
        }
        onAdd={() => setEditing('new')}
        onEdit={(p) => setEditing(p)}
        onDelete={(p) => deleteMutation.mutate({ id: p.id })}
        deleting={deleteMutation.isPending}
        onBind={(purpose, ids) => bindMutation.mutate({ purpose, profileId: ids[0] ?? '' })}
        binding={bindMutation.isPending}
      />

      <StorageProfileDrawer
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
