import { RobotOutlined, ThunderboltOutlined } from '@ant-design/icons'
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
import { App, Button, Form, Input } from 'antd'
import { useState } from 'react'
import { ProfileFormDrawer, type ProviderOption } from '#components/profile-form-drawer'
import { ProfileManager, ProviderTag } from '#components/profile-manager'
import {
  SchemaField,
  type SchemaProfileFormValues,
  schemaInitialConfig,
  schemaProfileToProto,
} from '#components/system/schema-profile-form'
import { TestAlert, type TestState } from '#components/system/test-alert'
import { humanizeError } from '#lib/errors'

const PURPOSE_LABELS: Record<string, () => string> = {
  chat: () => '通用对话',
}

const PROVIDER_NAMES: Record<string, () => string> = {
  openai: () => 'OpenAI 兼容',
  anthropic: () => 'Anthropic',
}

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
  const [editing, setEditing] = useState<Profile | 'new' | undefined>()

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

  const modelOf = (p: Profile) => String(p.config?.model ?? '')

  return (
    <>
      <ProfileManager
        profiles={profiles.map((p) => ({ ...p, name: p.name || (modelOf(p) ?? '') }))}
        bindings={bindings.map((b) => ({
          purpose: b.purpose,
          profileIds: b.profileId ? [b.profileId] : [],
        }))}
        texts={{
          profilesTitle: '模型配置',
          profilesHint: '可添加多个模型配置，例如高性价比的快速模型和更强的推理模型',
          noProfiles: '尚未添加模型配置',
          confirmDelete: '删除该存储配置？',
          bindingsHint: '为每个 AI 功能指定使用的模型配置，未绑定的功能将不可用',
        }}
        purposeLabel={(purpose) => PURPOSE_LABELS[purpose]?.() ?? purpose}
        profileIcon={(p) =>
          p.provider === 'anthropic' ? (
            <RobotOutlined className="text-lg text-(--ant-color-warning)" />
          ) : (
            <ThunderboltOutlined className="text-lg text-(--ant-color-primary)" />
          )
        }
        profileTags={(p) => <ProviderTag name={PROVIDER_NAMES[p.provider]?.() ?? p.provider} />}
        profileDescription={(p) => modelOf(p) || '模型'}
        onAdd={() => setEditing('new')}
        onEdit={(p) => setEditing(p)}
        onDelete={(p) => deleteMutation.mutate({ id: p.id })}
        deleting={deleteMutation.isPending}
        onBind={(purpose, ids) => bindMutation.mutate({ purpose, profileId: ids[0] ?? '' })}
        binding={bindMutation.isPending}
      />

      <LlmProfileDrawer
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
  const [form] = Form.useForm<SchemaProfileFormValues>()
  const [result, setResult] = useState<TestState>()

  const { data: describe } = useQuery(describeLlmProviders, {})
  const schemas = describe?.providers ?? {}

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

  const providers: ProviderOption[] = [
    {
      value: 'openai',
      label: 'OpenAI 兼容',
      description: '覆盖绝大多数模型服务：官方、DeepSeek、Qwen、自建等',
      icon: <ThunderboltOutlined className="text-xl text-(--ant-color-primary)" />,
    },
    {
      value: 'anthropic',
      label: 'Anthropic',
      description: 'Anthropic 原生接口',
      icon: <RobotOutlined className="text-xl text-(--ant-color-warning)" />,
    },
  ]

  const toProto = (provider: string, values: SchemaProfileFormValues) =>
    schemaProfileToProto(profile, provider, schemas[provider]?.fields ?? [], values)

  return (
    <ProfileFormDrawer
      open={open}
      onClose={onClose}
      form={form}
      profileProvider={profile?.provider}
      providers={providers}
    >
      {(provider) => {
        const fields = schemas[provider]?.fields ?? []
        return (
          <Form
            form={form}
            layout="vertical"
            requiredMark={false}
            initialValues={{
              name: profile?.name ?? '',
              config: schemaInitialConfig(profile, provider, fields),
            }}
            onFinish={(values) => {
              const p = toProto(provider, values)
              if (profile) updateMutation.mutate({ profile: p })
              else createMutation.mutate({ profile: p })
            }}
          >
            <Form.Item
              name="name"
              label={'配置名称'}
              rules={[{ required: true, message: '请输入配置名称' }]}
            >
              <Input placeholder={'如：快速模型、推理模型'} />
            </Form.Item>

            <div className="grid grid-cols-2 gap-4">
              {fields.map((field) => (
                <SchemaField key={field.key} field={field} profile={profile} />
              ))}
            </div>

            <TestAlert result={result} />
            <div className="flex gap-2">
              <Button
                type="primary"
                htmlType="submit"
                loading={createMutation.isPending || updateMutation.isPending}
              >
                {'保存'}
              </Button>
              <Button
                loading={testMutation.isPending}
                onClick={() => {
                  setResult(undefined)
                  testMutation.mutate({ profile: toProto(provider, form.getFieldsValue()) })
                }}
              >
                {'测试对话'}
              </Button>
            </div>
          </Form>
        )
      }}
    </ProfileFormDrawer>
  )
}
