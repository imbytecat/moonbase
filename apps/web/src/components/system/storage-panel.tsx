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

const PURPOSE_LABELS: Record<string, () => string> = {
  avatars: () => '用户头像',
  'site-assets': () => '站点资源',
}

const PROVIDER_NAMES: Record<string, () => string> = {
  local: () => '本地存储',
  s3: () => 'S3 兼容存储',
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
      message.success('存储配置已删除')
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const bindMutation = useMutation(bindStoragePurpose, {
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
          name: p.name || (p.provider === 'local' ? '本地存储' : String(p.config?.bucket ?? '')),
        }))}
        bindings={bindings.map((b) => ({
          purpose: b.purpose,
          profileIds: b.profileId ? [b.profileId] : [],
        }))}
        texts={{
          profilesTitle: '存储配置',
          profilesHint: '可添加多个存储配置，例如本地磁盘和云端存储桶，按用途绑定使用',
          noProfiles: '尚未添加存储配置',
          confirmDelete: '删除该存储配置？',
          bindingsHint: '为每类文件指定使用的存储配置，未绑定的功能将不可用',
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
                {p.config?.publicBaseUrl ? '公开' : '私有'}
              </Tag>
            ) : null}
          </>
        )}
        profileDescription={(p) =>
          p.provider === 'local'
            ? String(p.config?.directory || 'data/storage')
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
