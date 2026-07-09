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

const PURPOSE_LABELS: Record<string, () => string> = {
  auth: () => '登录与注册',
}

const PROVIDER_NAMES: Record<string, () => string> = {
  turnstile: () => 'Cloudflare Turnstile',
  geetest: () => '极验 v4',
  altcha: () => 'ALTCHA',
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
      message.success('存储配置已删除')
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const bindMutation = useMutation(bindCaptchaPurpose, {
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
        texts={{
          profilesTitle: '验证配置',
          profilesHint: '可添加多个人机验证配置，按用途选择启用',
          noProfiles: '尚未添加人机验证配置',
          confirmDelete: '删除该存储配置？',
          bindingsHint: '为每个场景指定使用的验证配置，未绑定的场景不启用人机验证',
        }}
        purposeLabel={(purpose) => PURPOSE_LABELS[purpose]?.() ?? purpose}
        profileIcon={() => <SafetyOutlined className="text-lg text-(--ant-color-primary)" />}
        profileTags={(p) => <ProviderTag name={PROVIDER_NAMES[p.provider]?.() ?? p.provider} />}
        profileDescription={(p) =>
          p.provider === 'altcha'
            ? '开源工作量证明验证，无需外部服务'
            : String(p.provider === 'geetest' ? p.config?.captchaId : p.config?.siteKey) ||
              '站点密钥'
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
      message.success('存储配置已创建')
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const updateMutation = useMutation(updateCaptchaProfile, {
    onSuccess: () => {
      message.success('设置已保存')
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const providers: ProviderOption[] = [
    {
      value: 'altcha',
      label: 'ALTCHA',
      description: '开源工作量证明验证，无需外部服务',
      icon: <ThunderboltOutlined className="text-xl text-(--ant-color-success)" />,
    },
    {
      value: 'turnstile',
      label: 'Cloudflare Turnstile',
      description: 'Cloudflare 提供的隐形人机验证',
      icon: <SafetyOutlined className="text-xl text-(--ant-color-warning)" />,
    },
    {
      value: 'geetest',
      label: '极验 v4',
      description: '极验第四代行为验证',
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
              label={'配置名称'}
              rules={[{ required: true, message: '请输入配置名称' }]}
            >
              <Input placeholder={'如：登录保护'} />
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
              {'保存'}
            </Button>
          </Form>
        )
      }}
    </ProfileFormDrawer>
  )
}
