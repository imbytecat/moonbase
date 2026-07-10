import { useMutation, useQuery } from '@connectrpc/connect-query'
import {
  bindStoragePurpose,
  deleteStorageProfile,
  describeStorageProviders,
  type Profile,
  type StorageSettings,
} from '@moonbase/api-client'
import { App } from 'antd'
import { ProfileManager } from '#components/profile-manager'
import { StorageProfileDrawer } from '#components/system/storage-profile-drawer'
import { humanizeError } from '#lib/errors'
import { useEditingTarget } from '#lib/use-editing-target'

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
  const drawer = useEditingTarget<Profile>()
  const { data: describe } = useQuery(describeStorageProviders, {})

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
        profiles={profiles}
        bindings={bindings.map((b) => ({
          purpose: b.purpose,
          profileIds: b.profileId ? [b.profileId] : [],
        }))}
        purposes={describe?.purposes ?? []}
        providers={describe?.providers ?? []}
        texts={{
          profilesTitle: '存储配置',
          profilesHint: '可添加多个存储配置，例如本地磁盘和云端存储桶，按用途绑定使用',
          noProfiles: '尚未添加存储配置',
          confirmDelete: '删除该存储配置？',
          bindingsHint: '为每类文件指定使用的存储配置，未绑定的功能将不可用',
        }}
        onAdd={drawer.add}
        onEdit={drawer.edit}
        onDelete={(p) => deleteMutation.mutate({ id: p.id })}
        deleting={deleteMutation.isPending}
        onBind={(purpose, ids) => bindMutation.mutate({ purpose, profileId: ids[0] ?? '' })}
        binding={bindMutation.isPending}
      />

      <StorageProfileDrawer
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
