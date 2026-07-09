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
import { m } from '#paraglide/messages.js'

const PURPOSE_LABELS: Record<string, () => string> = {
  chat: m.systemPage_llmPurposeChat,
}

const PROVIDER_NAMES: Record<string, () => string> = {
  openai: m.systemPage_llmOpenaiCompatible,
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
      message.success(m.systemPage_profileDeleted())
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const bindMutation = useMutation(bindLlmPurpose, {
    onSuccess: () => {
      onChanged()
      message.success(m.systemPage_saved())
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
          profilesTitle: m.systemPage_llmProfilesTitle(),
          profilesHint: m.systemPage_llmProfilesHint(),
          noProfiles: m.systemPage_llmNoProfiles(),
          confirmDelete: m.systemPage_confirmDeleteProfile(),
          bindingsHint: m.systemPage_llmBindingsHint(),
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
        profileDescription={(p) => modelOf(p) || m.systemPage_llmModel()}
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
      message.success(m.systemPage_profileCreated())
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const updateMutation = useMutation(updateLlmProfile, {
    onSuccess: () => {
      message.success(m.systemPage_saved())
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
      label: m.systemPage_llmOpenaiCompatible(),
      description: m.systemPage_llmOpenaiDesc(),
      icon: <ThunderboltOutlined className="text-xl text-(--ant-color-primary)" />,
    },
    {
      value: 'anthropic',
      label: 'Anthropic',
      description: m.systemPage_llmAnthropicDesc(),
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
              label={m.systemPage_profileName()}
              rules={[{ required: true, message: m.systemPage_profileNameRule() }]}
            >
              <Input placeholder={m.systemPage_llmProfileNamePlaceholder()} />
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
                {m.common_save()}
              </Button>
              <Button
                loading={testMutation.isPending}
                onClick={() => {
                  setResult(undefined)
                  testMutation.mutate({ profile: toProto(provider, form.getFieldsValue()) })
                }}
              >
                {m.systemPage_testLlm()}
              </Button>
            </div>
          </Form>
        )
      }}
    </ProfileFormDrawer>
  )
}
