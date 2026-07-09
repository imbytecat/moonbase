import { RadarChartOutlined, SafetyOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { useMutation, useQuery } from '@connectrpc/connect-query'
import {
  bindCaptchaPurpose,
  type CaptchaSettings,
  createCaptchaProfile,
  deleteCaptchaProfile,
  describeCaptchaProviders,
  type Profile,
  updateCaptchaProfile,
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
import { humanizeError } from '#lib/errors'
import { m } from '#paraglide/messages.js'

const PURPOSE_LABELS: Record<string, () => string> = {
  auth: m.systemPage_captchaPurposeAuth,
}

const PROVIDER_NAMES: Record<string, () => string> = {
  turnstile: () => 'Cloudflare Turnstile',
  geetest: m.systemPage_providerGeetest,
  altcha: m.systemPage_providerAltcha,
}

export function CaptchaPanel({
  captcha,
  onChanged,
}: {
  captcha: CaptchaSettings | undefined
  onChanged: () => void
}) {
  const { message } = App.useApp()
  const profiles = captcha?.profiles ?? []
  const bindings = captcha?.bindings ?? []
  const [editing, setEditing] = useState<Profile | 'new' | undefined>()

  const deleteMutation = useMutation(deleteCaptchaProfile, {
    onSuccess: () => {
      onChanged()
      message.success(m.systemPage_profileDeleted())
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const bindMutation = useMutation(bindCaptchaPurpose, {
    onSuccess: () => {
      onChanged()
      message.success(m.systemPage_saved())
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
        texts={{
          profilesTitle: m.systemPage_captchaProfilesTitle(),
          profilesHint: m.systemPage_captchaProfilesHint(),
          noProfiles: m.systemPage_captchaNoProfiles(),
          confirmDelete: m.systemPage_confirmDeleteProfile(),
          bindingsHint: m.systemPage_captchaBindingsHint(),
        }}
        purposeLabel={(purpose) => PURPOSE_LABELS[purpose]?.() ?? purpose}
        profileIcon={() => <SafetyOutlined className="text-lg text-(--ant-color-primary)" />}
        profileTags={(p) => <ProviderTag name={PROVIDER_NAMES[p.provider]?.() ?? p.provider} />}
        profileDescription={(p) =>
          p.provider === 'altcha'
            ? m.systemPage_altchaDesc()
            : String(p.provider === 'geetest' ? p.config?.captchaId : p.config?.siteKey) ||
              m.systemPage_siteKey()
        }
        onAdd={() => setEditing('new')}
        onEdit={(p) => setEditing(p)}
        onDelete={(p) => deleteMutation.mutate({ id: p.id })}
        deleting={deleteMutation.isPending}
        onBind={(purpose, ids) => bindMutation.mutate({ purpose, profileId: ids[0] ?? '' })}
        binding={bindMutation.isPending}
      />

      <CaptchaProfileDrawer
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

function CaptchaProfileDrawer({
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

  const { data: describe } = useQuery(describeCaptchaProviders, {})
  const schemas = describe?.providers ?? {}

  const createMutation = useMutation(createCaptchaProfile, {
    onSuccess: () => {
      message.success(m.systemPage_profileCreated())
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const updateMutation = useMutation(updateCaptchaProfile, {
    onSuccess: () => {
      message.success(m.systemPage_saved())
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const providers: ProviderOption[] = [
    {
      value: 'altcha',
      label: m.systemPage_providerAltcha(),
      description: m.systemPage_altchaDesc(),
      icon: <ThunderboltOutlined className="text-xl text-(--ant-color-success)" />,
    },
    {
      value: 'turnstile',
      label: 'Cloudflare Turnstile',
      description: m.systemPage_turnstileDesc(),
      icon: <SafetyOutlined className="text-xl text-(--ant-color-warning)" />,
    },
    {
      value: 'geetest',
      label: m.systemPage_providerGeetest(),
      description: m.systemPage_geetestDesc(),
      icon: <RadarChartOutlined className="text-xl text-(--ant-color-primary)" />,
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
              <Input placeholder={m.systemPage_captchaProfileNamePlaceholder()} />
            </Form.Item>

            <div className="grid grid-cols-2 gap-4">
              {fields.map((field) => (
                <SchemaField key={field.key} field={field} profile={profile} />
              ))}
            </div>

            <Button
              type="primary"
              htmlType="submit"
              loading={createMutation.isPending || updateMutation.isPending}
            >
              {m.common_save()}
            </Button>
          </Form>
        )
      }}
    </ProfileFormDrawer>
  )
}
