import { RobotOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { useMutation } from '@connectrpc/connect-query'
import {
  bindLlmPurpose,
  createLlmProfile,
  deleteLlmProfile,
  type LlmProfile,
  type LlmSettings,
  testLlm,
  updateLlmProfile,
} from '@moonbase/api-client'
import { App, Button, Form, Input } from 'antd'
import { useState } from 'react'
import { ProfileFormDrawer, type ProviderOption } from '#components/profile-form-drawer'
import { ProfileManager, ProviderTag } from '#components/profile-manager'
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
  const [editing, setEditing] = useState<LlmProfile | 'new' | undefined>()

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

  const modelOf = (p: LlmProfile) =>
    p.provider === 'anthropic' ? p.anthropic?.model : p.openai?.model

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

interface LlmProfileFormValues {
  name: string
  openai: { baseUrl: string; apiKey: string; model: string }
  anthropic: { baseUrl: string; apiKey: string; model: string }
}

function LlmProfileDrawer({
  profile,
  open,
  onClose,
  onChanged,
}: {
  profile: LlmProfile | undefined
  open: boolean
  onClose: () => void
  onChanged: () => void
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<LlmProfileFormValues>()
  const [result, setResult] = useState<TestState>()

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

  const storedOpenai = () => ({
    baseUrl: profile?.openai?.baseUrl ?? '',
    apiKey: '',
    model: profile?.openai?.model ?? '',
  })
  const storedAnthropic = () => ({
    baseUrl: profile?.anthropic?.baseUrl ?? '',
    apiKey: '',
    model: profile?.anthropic?.model ?? '',
  })

  const toProto = (provider: string, values: LlmProfileFormValues) => ({
    id: profile?.id ?? '',
    name: values.name ?? '',
    provider,
    openai: provider === 'openai' ? values.openai : storedOpenai(),
    anthropic: provider === 'anthropic' ? values.anthropic : storedAnthropic(),
  })

  const openaiSecretPlaceholder = profile?.openai?.apiKeySet ? m.systemPage_secretUnchanged() : ''
  const anthropicSecretPlaceholder = profile?.anthropic?.apiKeySet
    ? m.systemPage_secretUnchanged()
    : ''

  return (
    <ProfileFormDrawer
      open={open}
      onClose={onClose}
      form={form}
      profileProvider={profile?.provider}
      providers={providers}
    >
      {(provider) => (
        <Form
          form={form}
          layout="vertical"
          requiredMark={false}
          initialValues={{
            name: profile?.name ?? '',
            openai: storedOpenai(),
            anthropic: storedAnthropic(),
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
            <Form.Item
              name={[provider, 'baseUrl']}
              label={m.systemPage_llmBaseUrl()}
              extra={m.systemPage_llmBaseUrlHint()}
              className="col-span-2"
            >
              <Input
                placeholder={
                  provider === 'openai' ? 'https://api.openai.com/v1' : 'https://api.anthropic.com'
                }
              />
            </Form.Item>
            <Form.Item name={[provider, 'apiKey']} label="API Key">
              <Input.Password
                autoComplete="new-password"
                placeholder={
                  provider === 'openai' ? openaiSecretPlaceholder : anthropicSecretPlaceholder
                }
              />
            </Form.Item>
            <Form.Item name={[provider, 'model']} label={m.systemPage_llmModel()}>
              <Input placeholder={provider === 'openai' ? 'gpt-4o-mini' : 'claude-sonnet-4-5'} />
            </Form.Item>
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
      )}
    </ProfileFormDrawer>
  )
}
