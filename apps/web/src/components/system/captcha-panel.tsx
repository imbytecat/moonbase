import { RadarChartOutlined, SafetyOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { useMutation } from '@connectrpc/connect-query'
import {
  bindCaptchaPurpose,
  type CaptchaProfile,
  type CaptchaSettings,
  createCaptchaProfile,
  deleteCaptchaProfile,
  updateCaptchaProfile,
} from '@moonbase/api-client'
import { App, Button, Form, Input, InputNumber } from 'antd'
import { useState } from 'react'
import { ProfileFormDrawer, type ProviderOption } from '#components/profile-form-drawer'
import { ProfileManager, ProviderTag } from '#components/profile-manager'
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
  const [editing, setEditing] = useState<CaptchaProfile | 'new' | undefined>()

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
            : (p.provider === 'geetest' ? p.geetest?.captchaId : p.turnstile?.siteKey) ||
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

interface CaptchaProfileFormValues {
  name: string
  turnstile: { siteKey: string; secretKey: string }
  geetest: { captchaId: string; captchaKey: string }
  altcha: { difficulty: number }
}

function CaptchaProfileDrawer({
  profile,
  open,
  onClose,
  onChanged,
}: {
  profile: CaptchaProfile | undefined
  open: boolean
  onClose: () => void
  onChanged: () => void
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<CaptchaProfileFormValues>()

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

  const storedTurnstile = () => ({
    siteKey: profile?.turnstile?.siteKey ?? '',
    secretKey: '',
  })
  const storedGeetest = () => ({
    captchaId: profile?.geetest?.captchaId ?? '',
    captchaKey: '',
  })
  const storedAltcha = () => ({
    difficulty: profile?.altcha?.difficulty ?? 0,
  })

  const turnstileSecretPlaceholder = profile?.turnstile?.secretKeySet
    ? m.systemPage_secretUnchanged()
    : ''
  const geetestSecretPlaceholder = profile?.geetest?.captchaKeySet
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
            turnstile: storedTurnstile(),
            geetest: storedGeetest(),
            altcha: storedAltcha(),
          }}
          onFinish={(values: CaptchaProfileFormValues) => {
            const p = {
              id: profile?.id ?? '',
              name: values.name ?? '',
              provider,
              turnstile: provider === 'turnstile' ? values.turnstile : storedTurnstile(),
              geetest: provider === 'geetest' ? values.geetest : storedGeetest(),
              altcha: provider === 'altcha' ? values.altcha : storedAltcha(),
            }
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

          {provider === 'altcha' ? (
            <Form.Item
              name={['altcha', 'difficulty']}
              label={m.systemPage_altchaDifficulty()}
              extra={m.systemPage_altchaDifficultyHint()}
            >
              <InputNumber min={0} max={10000000} step={100000} className="w-full" />
            </Form.Item>
          ) : provider === 'turnstile' ? (
            <div className="grid grid-cols-2 gap-4">
              <Form.Item name={['turnstile', 'siteKey']} label={m.systemPage_siteKey()}>
                <Input autoComplete="off" />
              </Form.Item>
              <Form.Item name={['turnstile', 'secretKey']} label={m.systemPage_secretKey()}>
                <Input.Password
                  autoComplete="new-password"
                  placeholder={turnstileSecretPlaceholder}
                />
              </Form.Item>
            </div>
          ) : (
            <div className="grid grid-cols-2 gap-4">
              <Form.Item name={['geetest', 'captchaId']} label={m.systemPage_captchaId()}>
                <Input autoComplete="off" />
              </Form.Item>
              <Form.Item name={['geetest', 'captchaKey']} label={m.systemPage_captchaKey()}>
                <Input.Password
                  autoComplete="new-password"
                  placeholder={geetestSecretPlaceholder}
                />
              </Form.Item>
            </div>
          )}

          <Button
            type="primary"
            htmlType="submit"
            loading={createMutation.isPending || updateMutation.isPending}
          >
            {m.common_save()}
          </Button>
        </Form>
      )}
    </ProfileFormDrawer>
  )
}
