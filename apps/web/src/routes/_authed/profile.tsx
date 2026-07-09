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
    { key: 'profile', label: '基本资料', icon: <IdcardOutlined /> },
    { key: 'account', label: '账号绑定', icon: <LinkOutlined /> },
    { key: 'security', label: '安全设置', icon: <SafetyOutlined /> },
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
      message.success('资料已保存')
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const presignMutation = useMutation(presignAvatarUpload)

  const sendVerifyMutation = useMutation(sendVerificationEmail, {
    onSuccess: () => message.success('验证邮件已发送，请查收'),
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
      message.error(err instanceof Error ? humanizeError(err) : '请求失败，请稍后重试')
    }
  }

  return (
    <Card title={'个人资料'}>
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
          <Button loading={presignMutation.isPending}>{'更换头像'}</Button>
        </Upload>
      </div>

      <Form
        layout="vertical"
        requiredMark={false}
        initialValues={{ name: user?.name }}
        onFinish={(values: { name: string }) => profileMutation.mutate({ name: values.name })}
      >
        {user?.username ? (
          <Form.Item label={'用户名'}>
            <Input value={user.username} disabled />
          </Form.Item>
        ) : null}
        {user?.email ? (
          <Form.Item label={'邮箱'}>
            <div className="flex items-center gap-2">
              <Input value={user.email} disabled />
              {user.emailVerified ? (
                <Tag color="green">{'已验证'}</Tag>
              ) : (
                <>
                  <Tag>{'未验证'}</Tag>
                  {emailEnabled ? (
                    <Button
                      size="small"
                      loading={sendVerifyMutation.isPending}
                      onClick={() => sendVerifyMutation.mutate({})}
                    >
                      {'发送验证邮件'}
                    </Button>
                  ) : null}
                </>
              )}
            </div>
          </Form.Item>
        ) : null}
        <Form.Item name="name" label={'姓名'} rules={[{ required: true, message: '请输入姓名' }]}>
          <Input />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={profileMutation.isPending}>
          {'保存'}
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
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={'暂无可绑定的渠道——请先在系统设置中配置邮件、短信或第三方登录'}
        />
      </Card>
    )
  }
  return <>{cards}</>
}

function ChangePasswordCard() {
  const { message } = App.useApp()

  const passwordMutation = useMutation(changePassword, {
    onSuccess: () => message.success('密码已修改，其他设备已退出登录'),
    onError: (err) => message.error(humanizeError(err)),
  })

  return (
    <Card title={'修改密码'}>
      <Form
        layout="vertical"
        requiredMark={false}
        onFinish={(values: { currentPassword: string; newPassword: string }) =>
          passwordMutation.mutate(values)
        }
      >
        <Form.Item
          name="currentPassword"
          label={'当前密码'}
          rules={[{ required: true, min: 8, message: '密码至少 8 位' }]}
        >
          <Input.Password autoComplete="current-password" />
        </Form.Item>
        <Form.Item
          name="newPassword"
          label={'新密码'}
          rules={[{ required: true, min: 8, message: '密码至少 8 位' }]}
        >
          <Input.Password autoComplete="new-password" />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={passwordMutation.isPending}>
          {'修改密码'}
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
      message.success('验证码已发送')
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
      message.success('手机号绑定成功')
      form.resetFields()
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const unbindMutation = useMutation(unbindPhone, {
    onSuccess: () => {
      message.success('手机号已解绑')
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  return (
    <Card title={'手机绑定'}>
      {phone ? (
        <div className="space-y-3">
          <div className="flex items-center gap-2">
            <Input value={phone} disabled className="max-w-60" />
            <Tag color="green">{'已绑定'}</Tag>
          </div>
          {canUnbind ? (
            <UnbindForm
              label={'解绑手机'}
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
          <Form.Item name="phoneNumber" label={'手机号'} rules={[phoneRule()]}>
            <PhoneInput allowedRegions={allowedRegions} />
          </Form.Item>
          <Form.Item
            name="code"
            label={'验证码'}
            rules={[{ required: true, len: 6, message: '请输入 6 位验证码' }]}
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
                  {cooldown > 0 ? `${cooldown}秒后可重发` : '发送验证码'}
                </Button>
              }
            />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={bindMutation.isPending}>
            {'绑定手机'}
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
      message.success('验证码已发送')
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
      message.success('邮箱绑定成功')
      form.resetFields()
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const unbindMutation = useMutation(unbindEmail, {
    onSuccess: () => {
      message.success('邮箱已解绑')
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  if (email) {
    if (!canUnbind) return null
    return (
      <Card title={'邮箱绑定'}>
        <div className="space-y-3">
          <div className="flex items-center gap-2">
            <Input value={email} disabled className="max-w-60" />
            {verified ? <Tag color="green">{'已验证'}</Tag> : <Tag>{'未验证'}</Tag>}
          </div>
          <UnbindForm
            label={'解绑邮箱'}
            pending={unbindMutation.isPending}
            onSubmit={(currentPassword) => unbindMutation.mutate({ currentPassword })}
          />
        </div>
      </Card>
    )
  }
  if (!canBind) return null

  return (
    <Card title={'邮箱绑定'}>
      <Form
        form={form}
        layout="vertical"
        requiredMark={false}
        onFinish={(values) => bindMutation.mutate(values)}
      >
        <Form.Item
          name="email"
          label={'邮箱'}
          rules={[{ required: true, type: 'email', message: '请输入有效的邮箱地址' }]}
        >
          <Input autoComplete="email" />
        </Form.Item>
        <Form.Item
          name="code"
          label={'验证码'}
          rules={[{ required: true, len: 6, message: '请输入 6 位验证码' }]}
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
                {cooldown > 0 ? `${cooldown}秒后可重发` : '发送验证码'}
              </Button>
            }
          />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={bindMutation.isPending}>
          {'绑定邮箱'}
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
        rules={[{ required: true, min: 8, message: '密码至少 8 位' }]}
      >
        <Input.Password autoComplete="current-password" placeholder={'当前密码'} />
      </Form.Item>
      <Popconfirm
        title={'确定解绑该登录方式？'}
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
      message.success('设备已下线')
      void invalidate()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  return (
    <Card title={'已登录设备'}>
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
                {'当前设备'}
              </Tag>
            ) : (
              <Popconfirm
                title={'确定将该设备下线？'}
                okButtonProps={{ danger: true }}
                onConfirm={() => revokeMutation.mutate({ id: s.id })}
              >
                <Button size="small" danger loading={revokeMutation.isPending} className="shrink-0">
                  {'下线'}
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
      message.success('第三方账号已解绑')
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
    <Card title={'已绑定的第三方账号'}>
      {identities.length === 0 ? (
        <div className="py-4 text-center text-sm text-(--ant-color-text-tertiary)">
          {'尚未绑定第三方账号'}
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
                {'解绑'}
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
              {`绑定${opt.name || opt.key}`}
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
            rules={[{ required: true, min: 8, message: '密码至少 8 位' }]}
          >
            <Input.Password autoComplete="current-password" placeholder={'当前密码'} />
          </Form.Item>
          <Button danger htmlType="submit" loading={unbindMutation.isPending}>
            {'解绑'}
          </Button>
          <Button type="text" onClick={() => setUnbindTarget(undefined)}>
            {'取消'}
          </Button>
        </Form>
      ) : null}
    </Card>
  )
}

function describeUserAgent(ua: string): string {
  if (!ua) return '未知设备'
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
      message.success('两步验证已开启')
      setSetup(undefined)
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const disableMutation = useMutation(disableTotp, {
    onSuccess: () => {
      message.success('两步验证已关闭')
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  if (enabled) {
    return (
      <Card title={'两步验证'}>
        <div className="mb-3 flex items-center gap-2">
          <Tag color="green">{'已开启'}</Tag>
        </div>
        <Form
          form={disableForm}
          layout="inline"
          requiredMark={false}
          onFinish={({ currentPassword }) => disableMutation.mutate({ currentPassword })}
        >
          <Form.Item
            name="currentPassword"
            rules={[{ required: true, min: 8, message: '密码至少 8 位' }]}
          >
            <Input.Password autoComplete="current-password" placeholder={'当前密码'} />
          </Form.Item>
          <Popconfirm
            title={'确定关闭两步验证？'}
            okButtonProps={{ danger: true }}
            onConfirm={() => disableForm.submit()}
          >
            <Button danger loading={disableMutation.isPending}>
              {'关闭'}
            </Button>
          </Popconfirm>
        </Form>
      </Card>
    )
  }

  if (setup) {
    return (
      <Card title={'两步验证'}>
        <div className="space-y-4">
          <Alert
            type="warning"
            title={'请妥善保存以下恢复码——仅显示一次。身份验证器丢失时，每个恢复码可登录一次。'}
            showIcon
          />
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
              label={'输入身份验证器中的第一个验证码以确认'}
              rules={[{ required: true, len: 6, message: '请输入 6 位验证码' }]}
            >
              <Input maxLength={6} autoComplete="one-time-code" className="max-w-40" />
            </Form.Item>
            <div className="flex gap-2">
              <Button type="primary" htmlType="submit" loading={activateMutation.isPending}>
                {'确认并开启'}
              </Button>
              <Button onClick={() => setSetup(undefined)}>{'取消'}</Button>
            </div>
          </Form>
        </div>
      </Card>
    )
  }

  return (
    <Card title={'两步验证'}>
      <div className="mb-3 text-sm text-(--ant-color-text-secondary)">
        {'登录时除密码外，还需输入身份验证器 App（TOTP）中的动态验证码。'}
      </div>
      <Form
        form={setupForm}
        layout="inline"
        requiredMark={false}
        onFinish={({ currentPassword }) => setupMutation.mutate({ currentPassword })}
      >
        <Form.Item
          name="currentPassword"
          rules={[{ required: true, min: 8, message: '密码至少 8 位' }]}
        >
          <Input.Password autoComplete="current-password" placeholder={'当前密码'} />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={setupMutation.isPending}>
          {'开启'}
        </Button>
      </Form>
    </Card>
  )
}
