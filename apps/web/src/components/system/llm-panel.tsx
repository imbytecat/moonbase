import { useMutation, useQuery } from '@connectrpc/connect-query'
import {
  bindLlmPurpose,
  createLlmProfile,
  deleteLlmProfile,
  describeLlmProviders,
  type LlmSettings,
  type Profile,
  testLlm,
  updateLlmProfile,
} from '@moonbase/api-client'
import { App, Button } from 'antd'
import { useState } from 'react'
import { ProfileFormDrawer } from '#components/profile-form-drawer'
import { ProfileManager } from '#components/profile-manager'
import { ConfigForm } from '#components/system/config-form'
import { TestAlert, type TestState } from '#components/system/test-alert'
import { humanizeError } from '#lib/errors'
import { useEditingTarget } from '#lib/use-editing-target'

export function LlmPanel({
  llm,
  onChanged,
}: {
  llm: LlmSettings | undefined
  onChanged: () => void
}) {
  const { message } = App.useApp()
  const profiles = llm?.profiles ?? []
  const bindings = llm?.bindings ?? []
  const drawer = useEditingTarget<Profile>()
  const { data: describe } = useQuery(describeLlmProviders, {})

  const deleteMutation = useMutation(deleteLlmProfile, {
    onSuccess: () => {
      onChanged()
      message.success('存储配置已删除')
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const bindMutation = useMutation(bindLlmPurpose, {
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
          profilesTitle: '模型配置',
          profilesHint: '可添加多个模型配置，例如高性价比的快速模型和更强的推理模型',
          noProfiles: '尚未添加模型配置',
          confirmDelete: '删除该存储配置？',
          bindingsHint: '为每个 AI 功能指定使用的模型配置，未绑定的功能将不可用',
        }}
        onAdd={drawer.add}
        onEdit={drawer.edit}
        onDelete={(p) => deleteMutation.mutate({ id: p.id })}
        deleting={deleteMutation.isPending}
        onBind={(purpose, ids) => bindMutation.mutate({ purpose, profileId: ids[0] ?? '' })}
        binding={bindMutation.isPending}
      />

      <LlmProfileDrawer
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

function LlmProfileDrawer({
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
  const [result, setResult] = useState<TestState>()

  const { data: describe } = useQuery(describeLlmProviders, {})
  const providers = describe?.providers ?? []

  const createMutation = useMutation(createLlmProfile, {
    onSuccess: () => {
      message.success('存储配置已创建')
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const updateMutation = useMutation(updateLlmProfile, {
    onSuccess: () => {
      message.success('设置已保存')
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const testMutation = useMutation(testLlm, {
    onSuccess: (res) => setResult({ ok: res.ok, message: res.message }),
    onError: (err) => setResult({ ok: false, message: humanizeError(err) }),
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
            banner={() => <TestAlert result={result} />}
            actions={(current) => (
              <Button
                loading={testMutation.isPending}
                onClick={() => {
                  setResult(undefined)
                  testMutation.mutate({ profile: current })
                }}
              >
                {'测试对话'}
              </Button>
            )}
          />
        )
      }}
    </ProfileFormDrawer>
  )
}
