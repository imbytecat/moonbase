import { IdcardOutlined, LinkOutlined, SafetyOutlined } from '@ant-design/icons'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import {
  createConnectQueryKey,
  useMutation,
  useQuery,
  useSuspenseQuery,
} from '@connectrpc/connect-query'
import {
  activateTotp,
  bindEmail,
  bindPhone,
  type CurrentUser,
  changePassword,
  disableTotp,
  type GetAuthConfigResponse,
  getAuthConfig,
  getMe,
  listMyIdentities,
  listMySessions,
  type OauthProviderOption,
  presignAvatarUpload,
  revokeMySession,
  type SetupTotpResponse,
  sendEmailBindCode,
  sendPhoneBindCode,
  sendVerificationEmail,
  setupTotp,
  unbindEmail,
  unbindOauthIdentity,
  unbindPhone,
  updateProfile,
} from '@moonbase/api-client'
import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import {
  Alert,
  App,
  Avatar,
  Button,
  Card,
  Empty,
  Form,
  Input,
  Popconfirm,
  QRCode,
  Tag,
  Typography,
  Upload,
} from 'antd'
import { type ReactNode, useState } from 'react'
import { PhoneInput, phoneRule } from '#components/phone-input'
import { SectionNavLayout } from '#components/section-nav-layout'
import { humanizeError } from '#lib/errors'
import { sessionQueryKey } from '#lib/session'
import { uploadToPresignedUrl } from '#lib/upload'
import { m } from '#paraglide/messages.js'

export interface ProfileSearch {
  tab?: 'account' | 'security'
}

export const Route = createFileRoute('/_authed/profile')({
  validateSearch: (search: Record<string, unknown>): ProfileSearch => {
    const tab = search.tab
    return tab === 'account' || tab === 'security' ? { tab } : {}
  },
  component: ProfilePage,
})

type ProfileTab = 'profile' | 'account' | 'security'

function ProfilePage() {
  const queryClient = useQueryClient()
  const { data } = useSuspenseQuery(getMe)
  const { data: authConfig } = useQuery(getAuthConfig)
  const { tab } = Route.useSearch()
  const navigate = Route.useNavigate()
  const user = data.user
  const active: ProfileTab = tab ?? 'profile'

  const setTab = (next: ProfileTab) =>
    void navigate({ search: next === 'profile' ? {} : { tab: next }, replace: true })

  const invalidateSession = () => queryClient.invalidateQueries({ queryKey: sessionQueryKey() })

  const sections: { key: ProfileTab; label: string; icon: ReactNode }[] = [
    { key: 'profile', label: m.profile_sectionProfile(), icon: <IdcardOutlined /> },
    { key: 'account', label: m.profile_sectionAccount(), icon: <LinkOutlined /> },
    { key: 'security', label: m.profile_sectionSecurity(), icon: <SafetyOutlined /> },
  ]

  return (
    <div className="mx-auto max-w-5xl">
      <SectionNavLayout
        groups={[{ key: 'profile-sections', items: sections }]}
        selectedKey={active}
        onSelect={(key) => setTab(key as ProfileTab)}
      >
        <div className="space-y-6">
          {active === 'profile' ? (
            <ProfileBasicsCard
              user={user}
              emailEnabled={authConfig?.emailEnabled ?? false}
              onChanged={() => void invalidateSession()}
            />
          ) : null}

          {active === 'account' ? (
            <AccountBindings
              user={user}
              authConfig={authConfig}
              onChanged={() => void invalidateSession()}
            />
          ) : null}

          {active === 'security' ? (
            <>
              <ChangePasswordCard />
              <TotpCard
                enabled={user?.totpEnabled ?? false}
                onChanged={() => void invalidateSession()}
              />
              <SessionsCard />
            </>
          ) : null}
        </div>
      </SectionNavLayout>
    </div>
  )
}

function ProfileBasicsCard({
  user,
  emailEnabled,
  onChanged,
}: {
  user: CurrentUser | undefined
  emailEnabled: boolean
  onChanged: () => void
}) {
  const { message } = App.useApp()

  const profileMutation = useMutation(updateProfile, {
    onSuccess: () => {
      onChanged()
      message.success(m.profile_profileUpdated())
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const presignMutation = useMutation(presignAvatarUpload)

  const sendVerifyMutation = useMutation(sendVerificationEmail, {
    onSuccess: () => message.success(m.profile_verifyEmailSent()),
    onError: (err) => message.error(humanizeError(err)),
  })

  const uploadAvatar = async (file: File) => {
    try {
      const presigned = await presignMutation.mutateAsync({
        contentType: file.type,
        contentLength: BigInt(file.size),
      })
      await uploadToPresignedUrl(presigned.uploadUrl, file)
      profileMutation.mutate({ avatarFileId: presigned.fileId })
    } catch (err) {
      message.error(err instanceof Error ? humanizeError(err) : m.error_generic())
    }
  }

  return (
    <Card title={m.profile_title()}>
      <div className="mb-6 flex items-center gap-4">
        <Avatar size={64} src={user?.avatarUrl || undefined}>
          {user?.name?.charAt(0).toUpperCase()}
        </Avatar>
        <Upload
          accept="image/jpeg,image/png,image/webp"
          showUploadList={false}
          beforeUpload={(file) => {
            void uploadAvatar(file)
            return false
          }}
        >
          <Button loading={presignMutation.isPending}>{m.profile_changeAvatar()}</Button>
        </Upload>
      </div>

      <Form
        layout="vertical"
        requiredMark={false}
        initialValues={{ name: user?.name }}
        onFinish={(values: { name: string }) => profileMutation.mutate({ name: values.name })}
      >
        {user?.username ? (
          <Form.Item label={m.auth_username()}>
            <Input value={user.username} disabled />
          </Form.Item>
        ) : null}
        {user?.email ? (
          <Form.Item label={m.auth_email()}>
            <div className="flex items-center gap-2">
              <Input value={user.email} disabled />
              {user.emailVerified ? (
                <Tag color="green">{m.profile_emailVerified()}</Tag>
              ) : (
                <>
                  <Tag>{m.profile_emailUnverified()}</Tag>
                  {emailEnabled ? (
                    <Button
                      size="small"
                      loading={sendVerifyMutation.isPending}
                      onClick={() => sendVerifyMutation.mutate({})}
                    >
                      {m.profile_sendVerifyEmail()}
                    </Button>
                  ) : null}
                </>
              )}
            </div>
          </Form.Item>
        ) : null}
        <Form.Item
          name="name"
          label={m.auth_name()}
          rules={[{ required: true, message: m.auth_nameRule() }]}
        >
          <Input />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={profileMutation.isPending}>
          {m.common_save()}
        </Button>
      </Form>
    </Card>
  )
}

function AccountBindings({
  user,
  authConfig,
  onChanged,
}: {
  user: CurrentUser | undefined
  authConfig: GetAuthConfigResponse | undefined
  onChanged: () => void
}) {
  const cards = [
    authConfig?.emailEnabled || user?.email ? (
      <EmailCard
        key="email"
        email={user?.email ?? ''}
        verified={user?.emailVerified ?? false}
        canBind={authConfig?.emailEnabled ?? false}
        canUnbind={Boolean(user?.username || user?.phone)}
        onChanged={onChanged}
      />
    ) : null,
    authConfig?.smsEnabled ? (
      <PhoneCard
        key="phone"
        phone={user?.phone ?? ''}
        allowedRegions={authConfig?.allowedPhoneRegions ?? []}
        canUnbind={Boolean(user?.username || user?.email)}
        onChanged={onChanged}
      />
    ) : null,
    (authConfig?.oauthProviders ?? []).length > 0 ? (
      <IdentitiesCard key="identities" options={authConfig?.oauthProviders ?? []} />
    ) : null,
  ].filter(Boolean)

  if (cards.length === 0) {
    return (
      <Card>
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={m.profile_noBindingChannels()} />
      </Card>
    )
  }
  return <>{cards}</>
}

function ChangePasswordCard() {
  const { message } = App.useApp()

  const passwordMutation = useMutation(changePassword, {
    onSuccess: () => message.success(m.profile_passwordChanged()),
    onError: (err) => message.error(humanizeError(err)),
  })

  return (
    <Card title={m.profile_changePasswordTitle()}>
      <Form
        layout="vertical"
        requiredMark={false}
        onFinish={(values: { currentPassword: string; newPassword: string }) =>
          passwordMutation.mutate(values)
        }
      >
        <Form.Item
          name="currentPassword"
          label={m.profile_currentPassword()}
          rules={[{ required: true, min: 8, message: m.auth_passwordRule() }]}
        >
          <Input.Password autoComplete="current-password" />
        </Form.Item>
        <Form.Item
          name="newPassword"
          label={m.profile_newPassword()}
          rules={[{ required: true, min: 8, message: m.auth_passwordRule() }]}
        >
          <Input.Password autoComplete="new-password" />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={passwordMutation.isPending}>
          {m.profile_changePassword()}
        </Button>
      </Form>
    </Card>
  )
}

function PhoneCard({
  phone,
  allowedRegions,
  canUnbind,
  onChanged,
}: {
  phone: string
  allowedRegions: string[]
  canUnbind: boolean
  onChanged: () => void
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<{ phoneNumber: string; code: string }>()
  const [cooldown, setCooldown] = useState(0)

  const sendCode = useMutation(sendPhoneBindCode, {
    onSuccess: () => {
      message.success(m.auth_codeSent())
      setCooldown(60)
      const timer = setInterval(() => {
        setCooldown((s) => {
          if (s <= 1) clearInterval(timer)
          return s - 1
        })
      }, 1000)
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const bindMutation = useMutation(bindPhone, {
    onSuccess: () => {
      message.success(m.profile_phoneBindSuccess())
      form.resetFields()
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const unbindMutation = useMutation(unbindPhone, {
    onSuccess: () => {
      message.success(m.profile_phoneUnbound())
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  return (
    <Card title={m.profile_phoneTitle()}>
      {phone ? (
        <div className="space-y-3">
          <div className="flex items-center gap-2">
            <Input value={phone} disabled className="max-w-60" />
            <Tag color="green">{m.profile_phoneBound()}</Tag>
          </div>
          {canUnbind ? (
            <UnbindForm
              label={m.profile_unbindPhone()}
              pending={unbindMutation.isPending}
              onSubmit={(currentPassword) => unbindMutation.mutate({ currentPassword })}
            />
          ) : null}
        </div>
      ) : (
        <Form
          form={form}
          layout="vertical"
          requiredMark={false}
          onFinish={(values) => bindMutation.mutate(values)}
        >
          <Form.Item name="phoneNumber" label={m.auth_phone()} rules={[phoneRule()]}>
            <PhoneInput allowedRegions={allowedRegions} />
          </Form.Item>
          <Form.Item
            name="code"
            label={m.auth_code()}
            rules={[{ required: true, len: 6, message: m.auth_codeRule() }]}
          >
            <Input
              maxLength={6}
              autoComplete="one-time-code"
              addonAfter={
                <Button
                  size="small"
                  type="text"
                  loading={sendCode.isPending}
                  disabled={cooldown > 0}
                  onClick={() => {
                    void form.validateFields(['phoneNumber']).then(({ phoneNumber }) => {
                      sendCode.mutate({ phoneNumber })
                    })
                  }}
                >
                  {cooldown > 0 ? m.auth_resendIn({ seconds: cooldown }) : m.auth_sendCode()}
                </Button>
              }
            />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={bindMutation.isPending}>
            {m.profile_bindPhone()}
          </Button>
        </Form>
      )}
    </Card>
  )
}

function EmailCard({
  email,
  verified,
  canBind,
  canUnbind,
  onChanged,
}: {
  email: string
  verified: boolean
  canBind: boolean
  canUnbind: boolean
  onChanged: () => void
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<{ email: string; code: string }>()
  const [cooldown, setCooldown] = useState(0)

  const sendCode = useMutation(sendEmailBindCode, {
    onSuccess: () => {
      message.success(m.auth_codeSent())
      setCooldown(60)
      const timer = setInterval(() => {
        setCooldown((s) => {
          if (s <= 1) clearInterval(timer)
          return s - 1
        })
      }, 1000)
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const bindMutation = useMutation(bindEmail, {
    onSuccess: () => {
      message.success(m.profile_emailBindSuccess())
      form.resetFields()
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const unbindMutation = useMutation(unbindEmail, {
    onSuccess: () => {
      message.success(m.profile_emailUnbound())
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  if (email) {
    if (!canUnbind) return null
    return (
      <Card title={m.profile_emailTitle()}>
        <div className="space-y-3">
          <div className="flex items-center gap-2">
            <Input value={email} disabled className="max-w-60" />
            {verified ? (
              <Tag color="green">{m.profile_emailVerified()}</Tag>
            ) : (
              <Tag>{m.profile_emailUnverified()}</Tag>
            )}
          </div>
          <UnbindForm
            label={m.profile_unbindEmail()}
            pending={unbindMutation.isPending}
            onSubmit={(currentPassword) => unbindMutation.mutate({ currentPassword })}
          />
        </div>
      </Card>
    )
  }
  if (!canBind) return null

  return (
    <Card title={m.profile_emailTitle()}>
      <Form
        form={form}
        layout="vertical"
        requiredMark={false}
        onFinish={(values) => bindMutation.mutate(values)}
      >
        <Form.Item
          name="email"
          label={m.auth_email()}
          rules={[{ required: true, type: 'email', message: m.auth_emailRule() }]}
        >
          <Input autoComplete="email" />
        </Form.Item>
        <Form.Item
          name="code"
          label={m.auth_code()}
          rules={[{ required: true, len: 6, message: m.auth_codeRule() }]}
        >
          <Input
            maxLength={6}
            autoComplete="one-time-code"
            addonAfter={
              <Button
                size="small"
                type="text"
                loading={sendCode.isPending}
                disabled={cooldown > 0}
                onClick={() => {
                  void form.validateFields(['email']).then(({ email }) => {
                    sendCode.mutate({ email })
                  })
                }}
              >
                {cooldown > 0 ? m.auth_resendIn({ seconds: cooldown }) : m.auth_sendCode()}
              </Button>
            }
          />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={bindMutation.isPending}>
          {m.profile_bindEmail()}
        </Button>
      </Form>
    </Card>
  )
}

function UnbindForm({
  label,
  pending,
  onSubmit,
}: {
  label: string
  pending: boolean
  onSubmit: (currentPassword: string) => void
}) {
  const [form] = Form.useForm<{ currentPassword: string }>()

  return (
    <Form
      form={form}
      layout="inline"
      requiredMark={false}
      onFinish={({ currentPassword }) => onSubmit(currentPassword)}
    >
      <Form.Item
        name="currentPassword"
        rules={[{ required: true, min: 8, message: m.auth_passwordRule() }]}
      >
        <Input.Password autoComplete="current-password" placeholder={m.profile_currentPassword()} />
      </Form.Item>
      <Popconfirm
        title={m.profile_unbindConfirm()}
        okButtonProps={{ danger: true }}
        onConfirm={() => form.submit()}
      >
        <Button danger loading={pending}>
          {label}
        </Button>
      </Popconfirm>
    </Form>
  )
}

function SessionsCard() {
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const { data } = useQuery(listMySessions)

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listMySessions, cardinality: 'finite' }),
    })

  const revokeMutation = useMutation(revokeMySession, {
    onSuccess: () => {
      message.success(m.profile_sessionRevoked())
      void invalidate()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  return (
    <Card title={m.profile_sessionsTitle()}>
      <ul className="m-0 list-none divide-y divide-(--ant-color-split) p-0">
        {(data?.sessions ?? []).map((s) => (
          <li key={s.id} className="flex flex-wrap items-center gap-3 py-3">
            <div className="min-w-0 flex-1 basis-52">
              <div className="font-medium">{describeUserAgent(s.userAgent)}</div>
              <div className="text-sm text-(--ant-color-text-tertiary)">
                {[s.ip, s.createdAt ? timestampDate(s.createdAt).toLocaleString() : '']
                  .filter(Boolean)
                  .join(' · ')}
              </div>
            </div>
            {s.current ? (
              <Tag color="blue" className="!me-0 shrink-0">
                {m.profile_currentDevice()}
              </Tag>
            ) : (
              <Popconfirm
                title={m.profile_revokeConfirm()}
                okButtonProps={{ danger: true }}
                onConfirm={() => revokeMutation.mutate({ id: s.id })}
              >
                <Button size="small" danger loading={revokeMutation.isPending} className="shrink-0">
                  {m.profile_revokeSession()}
                </Button>
              </Popconfirm>
            )}
          </li>
        ))}
      </ul>
    </Card>
  )
}

function IdentitiesCard({ options }: { options: OauthProviderOption[] }) {
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const { data } = useQuery(listMyIdentities)
  const [unbindTarget, setUnbindTarget] = useState<string>()
  const [form] = Form.useForm<{ currentPassword: string }>()

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listMyIdentities, cardinality: 'finite' }),
    })

  const unbindMutation = useMutation(unbindOauthIdentity, {
    onSuccess: () => {
      message.success(m.profile_identityUnbound())
      setUnbindTarget(undefined)
      void invalidate()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const identities = data?.identities ?? []
  const bound = new Set(identities.map((i) => i.providerKey))
  const labelOf = (key: string) => options.find((o) => o.key === key)?.name || key
  const unboundOptions = options.filter((o) => !bound.has(o.key))

  return (
    <Card title={m.profile_identitiesTitle()}>
      {identities.length === 0 ? (
        <div className="py-4 text-center text-sm text-(--ant-color-text-tertiary)">
          {m.profile_noIdentities()}
        </div>
      ) : (
        <ul className="m-0 list-none divide-y divide-(--ant-color-split) p-0">
          {identities.map((identity) => (
            <li key={identity.providerKey} className="flex flex-wrap items-center gap-3 py-3">
              <div className="min-w-0 flex-1 basis-52">
                <div className="font-medium">{labelOf(identity.providerKey)}</div>
                <div className="text-sm text-(--ant-color-text-tertiary)">
                  {[
                    identity.name,
                    identity.createdAt
                      ? timestampDate(identity.createdAt).toLocaleDateString()
                      : '',
                  ]
                    .filter(Boolean)
                    .join(' · ')}
                </div>
              </div>
              <Button
                size="small"
                danger
                className="shrink-0"
                onClick={() => {
                  form.resetFields()
                  setUnbindTarget(identity.providerKey)
                }}
              >
                {m.profile_unbindIdentity()}
              </Button>
            </li>
          ))}
        </ul>
      )}
      {unboundOptions.length > 0 ? (
        <div className="mt-3 flex flex-wrap gap-2">
          {unboundOptions.map((opt) => (
            <Button
              key={opt.key}
              onClick={() => {
                window.location.href = `/api/oauth/${encodeURIComponent(opt.key)}/authorize`
              }}
            >
              {m.profile_bindIdentity({ name: opt.name || opt.key })}
            </Button>
          ))}
        </div>
      ) : null}

      {unbindTarget ? (
        <Form
          form={form}
          layout="inline"
          requiredMark={false}
          className="mt-3"
          onFinish={({ currentPassword }) =>
            unbindMutation.mutate({ providerKey: unbindTarget, currentPassword })
          }
        >
          <Form.Item
            name="currentPassword"
            rules={[{ required: true, min: 8, message: m.auth_passwordRule() }]}
          >
            <Input.Password
              autoComplete="current-password"
              placeholder={m.profile_currentPassword()}
            />
          </Form.Item>
          <Button danger htmlType="submit" loading={unbindMutation.isPending}>
            {m.profile_unbindIdentity()}
          </Button>
          <Button type="text" onClick={() => setUnbindTarget(undefined)}>
            {m.common_cancel()}
          </Button>
        </Form>
      ) : null}
    </Card>
  )
}

function describeUserAgent(ua: string): string {
  if (!ua) return m.profile_unknownDevice()
  const browser = /Edg\//.test(ua)
    ? 'Edge'
    : /Chrome\//.test(ua)
      ? 'Chrome'
      : /Firefox\//.test(ua)
        ? 'Firefox'
        : /Safari\//.test(ua)
          ? 'Safari'
          : ua.slice(0, 40)
  const os = /Windows/.test(ua)
    ? 'Windows'
    : /Mac OS X/.test(ua)
      ? 'macOS'
      : /Android/.test(ua)
        ? 'Android'
        : /iPhone|iPad/.test(ua)
          ? 'iOS'
          : /Linux/.test(ua)
            ? 'Linux'
            : ''
  return os ? `${browser} · ${os}` : browser
}

function TotpCard({ enabled, onChanged }: { enabled: boolean; onChanged: () => void }) {
  const { message } = App.useApp()
  const [setup, setSetup] = useState<SetupTotpResponse>()
  const [setupForm] = Form.useForm<{ currentPassword: string }>()
  const [activateForm] = Form.useForm<{ code: string }>()
  const [disableForm] = Form.useForm<{ currentPassword: string }>()

  const setupMutation = useMutation(setupTotp, {
    onSuccess: (res) => {
      setSetup(res)
      setupForm.resetFields()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const activateMutation = useMutation(activateTotp, {
    onSuccess: () => {
      message.success(m.profile_totpEnabled())
      setSetup(undefined)
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const disableMutation = useMutation(disableTotp, {
    onSuccess: () => {
      message.success(m.profile_totpDisabled())
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  if (enabled) {
    return (
      <Card title={m.profile_totpTitle()}>
        <div className="mb-3 flex items-center gap-2">
          <Tag color="green">{m.profile_totpActive()}</Tag>
        </div>
        <Form
          form={disableForm}
          layout="inline"
          requiredMark={false}
          onFinish={({ currentPassword }) => disableMutation.mutate({ currentPassword })}
        >
          <Form.Item
            name="currentPassword"
            rules={[{ required: true, min: 8, message: m.auth_passwordRule() }]}
          >
            <Input.Password
              autoComplete="current-password"
              placeholder={m.profile_currentPassword()}
            />
          </Form.Item>
          <Popconfirm
            title={m.profile_totpDisableConfirm()}
            okButtonProps={{ danger: true }}
            onConfirm={() => disableForm.submit()}
          >
            <Button danger loading={disableMutation.isPending}>
              {m.profile_totpDisable()}
            </Button>
          </Popconfirm>
        </Form>
      </Card>
    )
  }

  if (setup) {
    return (
      <Card title={m.profile_totpTitle()}>
        <div className="space-y-4">
          <Alert type="warning" title={m.profile_totpRecoveryHint()} showIcon />
          <div className="flex justify-center rounded bg-white p-3">
            <QRCode value={setup.otpauthUrl} />
          </div>
          <Typography.Paragraph className="text-center" copyable={{ text: setup.secret }}>
            {setup.secret}
          </Typography.Paragraph>
          <div className="grid grid-cols-2 gap-1 rounded bg-(--ant-color-fill-quaternary) p-3 font-mono text-sm">
            {setup.recoveryCodes.map((code) => (
              <div key={code}>{code}</div>
            ))}
          </div>
          <Form
            form={activateForm}
            layout="vertical"
            requiredMark={false}
            onFinish={({ code }) => activateMutation.mutate({ code })}
          >
            <Form.Item
              name="code"
              label={m.profile_totpFirstCode()}
              rules={[{ required: true, len: 6, message: m.auth_codeRule() }]}
            >
              <Input maxLength={6} autoComplete="one-time-code" className="max-w-40" />
            </Form.Item>
            <div className="flex gap-2">
              <Button type="primary" htmlType="submit" loading={activateMutation.isPending}>
                {m.profile_totpActivate()}
              </Button>
              <Button onClick={() => setSetup(undefined)}>{m.common_cancel()}</Button>
            </div>
          </Form>
        </div>
      </Card>
    )
  }

  return (
    <Card title={m.profile_totpTitle()}>
      <div className="mb-3 text-sm text-(--ant-color-text-secondary)">{m.profile_totpHint()}</div>
      <Form
        form={setupForm}
        layout="inline"
        requiredMark={false}
        onFinish={({ currentPassword }) => setupMutation.mutate({ currentPassword })}
      >
        <Form.Item
          name="currentPassword"
          rules={[{ required: true, min: 8, message: m.auth_passwordRule() }]}
        >
          <Input.Password
            autoComplete="current-password"
            placeholder={m.profile_currentPassword()}
          />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={setupMutation.isPending}>
          {m.profile_totpSetup()}
        </Button>
      </Form>
    </Card>
  )
}
