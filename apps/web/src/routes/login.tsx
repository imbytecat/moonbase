import { LoginOutlined, WechatOutlined } from '@ant-design/icons'
import { createQueryOptions, useMutation, useQuery } from '@connectrpc/connect-query'
import {
  getAuthConfig,
  login,
  loginWithSms,
  loginWithTotp,
  sendSmsLoginCode,
} from '@moonbase/api-client'
import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute, Link, redirect, useRouter } from '@tanstack/react-router'
import { Alert, App, Button, Divider, Form, Input, Tabs, Typography } from 'antd'
import type { ReactNode } from 'react'
import { useState } from 'react'
import { AuthShell } from '#components/auth-shell'
import { CaptchaWidget } from '#components/captcha-widget'
import { PhoneInput, phoneRule } from '#components/phone-input'
import { humanizeError } from '#lib/errors'
import { sessionQueryOptions } from '#lib/session'

export interface LoginSearch {
  redirect?: string
  oauthError?: string
}

export const Route = createFileRoute('/login')({
  validateSearch: (search: Record<string, unknown>): LoginSearch => ({
    redirect: typeof search.redirect === 'string' ? search.redirect : undefined,
    oauthError: typeof search.oauthError === 'string' ? search.oauthError : undefined,
  }),
  beforeLoad: async ({ context: { queryClient, transport }, search }) => {
    const session = await queryClient
      .fetchQuery(sessionQueryOptions(transport))
      .catch(() => undefined)
    if (session?.user) throw redirect({ to: search.redirect ?? '/' })
  },
  loader: ({ context: { queryClient, transport } }) =>
    queryClient.ensureQueryData(createQueryOptions(getAuthConfig, undefined, { transport })),
  component: LoginPage,
})

function LoginPage() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const search = Route.useSearch()
  const { data: authConfig } = useQuery(getAuthConfig)
  const [error, setError] = useState<string | undefined>(search.oauthError)
  const [captchaToken, setCaptchaToken] = useState('')
  const [mfaTicket, setMfaTicket] = useState('')

  const captchaRequired = Boolean(authConfig?.captchaProvider)

  const enterApp = async () => {
    queryClient.clear()
    await router.invalidate()
    await router.navigate({ to: search.redirect ?? '/' })
  }

  const passwordLogin = useMutation(login, {
    onSuccess: async (res) => {
      if (res.mfaRequired) {
        setMfaTicket(res.mfaTicket)
        return
      }
      await enterApp()
    },
    onError: (err) => {
      setCaptchaToken('')
      setError(humanizeError(err))
    },
  })

  const totpLogin = useMutation(loginWithTotp, {
    onSuccess: enterApp,
    onError: (err) => setError(humanizeError(err)),
  })

  const smsLogin = useMutation(loginWithSms, {
    onSuccess: enterApp,
    onError: (err) => setError(humanizeError(err)),
  })

  const captcha = captchaRequired ? (
    <CaptchaWidget
      provider={authConfig?.captchaProvider ?? ''}
      siteKey={authConfig?.captchaSiteKey ?? ''}
      onToken={setCaptchaToken}
    />
  ) : null

  const passwordForm = (
    <Form
      layout="vertical"
      requiredMark={false}
      disabled={passwordLogin.isPending}
      onFinish={(values: { identifier: string; password: string }) => {
        setError(undefined)
        passwordLogin.mutate({ ...values, captchaToken })
      }}
    >
      <Form.Item
        name="identifier"
        label={'用户名 / 邮箱 / 手机号'}
        rules={[{ required: true, message: '请输入用户名、邮箱或手机号' }]}
      >
        <Input autoComplete="username" />
      </Form.Item>
      <Form.Item name="password" label={'密码'} rules={[{ required: true, message: '请输入密码' }]}>
        <Input.Password autoComplete="current-password" />
      </Form.Item>
      {authConfig?.emailEnabled ? (
        <div className="mb-3 text-end text-sm">
          <Link to="/forgot-password">{'忘记密码？'}</Link>
        </div>
      ) : null}
      {captcha}
      <Button
        type="primary"
        htmlType="submit"
        block
        loading={passwordLogin.isPending}
        disabled={captchaRequired && !captchaToken}
      >
        {'登录'}
      </Button>
    </Form>
  )

  return (
    <AuthShell subtitle={'登录账号'}>
      {error ? <Alert type="error" title={error} className="mb-4" showIcon /> : null}

      {mfaTicket ? (
        <Form
          layout="vertical"
          requiredMark={false}
          disabled={totpLogin.isPending}
          onFinish={(values: { code: string }) => {
            setError(undefined)
            totpLogin.mutate({ mfaTicket, code: values.code })
          }}
        >
          <Alert
            type="info"
            title={'请输入身份验证器 App 中的 6 位验证码，或恢复码。'}
            className="mb-4"
            showIcon
          />
          <Form.Item
            name="code"
            label={'动态验证码'}
            rules={[{ required: true, min: 6, message: '请输入 6 位验证码' }]}
          >
            <Input autoComplete="one-time-code" autoFocus maxLength={32} />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={totpLogin.isPending}>
            {'登录'}
          </Button>
          <Button
            type="text"
            block
            className="mt-2"
            onClick={() => {
              setMfaTicket('')
              setError(undefined)
            }}
          >
            {'返回登录'}
          </Button>
        </Form>
      ) : (
        <>
          {authConfig?.smsEnabled ? (
            <Tabs
              centered
              items={[
                { key: 'password', label: '密码登录', children: passwordForm },
                {
                  key: 'sms',
                  label: '短信登录',
                  children: (
                    <SmsLoginForm
                      captchaToken={captchaToken}
                      captchaRequired={captchaRequired}
                      captcha={captcha}
                      allowedRegions={authConfig?.allowedPhoneRegions ?? []}
                      onError={setError}
                      onSubmit={(values) => {
                        setError(undefined)
                        smsLogin.mutate(values)
                      }}
                      submitting={smsLogin.isPending}
                    />
                  ),
                },
              ]}
            />
          ) : (
            passwordForm
          )}

          {(authConfig?.oauthProviders ?? []).length > 0 ? (
            <>
              <Divider plain className="text-xs text-(--ant-color-text-tertiary)">
                {'或使用以下方式登录'}
              </Divider>
              <div className="flex flex-wrap justify-center gap-3">
                {authConfig?.oauthProviders?.map((opt) => (
                  <Button
                    key={opt.key}
                    icon={opt.provider === 'wechat' ? <WechatOutlined /> : <LoginOutlined />}
                    onClick={() => {
                      window.location.href = `/api/oauth/${encodeURIComponent(opt.key)}/authorize`
                    }}
                  >
                    {opt.name || opt.key}
                  </Button>
                ))}
              </div>
            </>
          ) : null}

          {authConfig?.registrationEnabled ? (
            <div className="mt-4 text-center text-sm">
              <Typography.Text type="secondary">{'还没有账号？'} </Typography.Text>
              <Link to="/register">{'立即注册'}</Link>
            </div>
          ) : null}
        </>
      )}
    </AuthShell>
  )
}

function SmsLoginForm({
  captchaToken,
  captchaRequired,
  captcha,
  allowedRegions,
  onError,
  onSubmit,
  submitting,
}: {
  captchaToken: string
  captchaRequired: boolean
  captcha: ReactNode
  allowedRegions: string[]
  onError: (msg: string) => void
  onSubmit: (values: { phoneNumber: string; code: string }) => void
  submitting: boolean
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<{ phoneNumber: string; code: string }>()
  const [cooldown, setCooldown] = useState(0)

  const sendCode = useMutation(sendSmsLoginCode, {
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
    onError: (err) => onError(humanizeError(err)),
  })

  return (
    <Form form={form} layout="vertical" requiredMark={false} onFinish={onSubmit}>
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
              disabled={cooldown > 0 || (captchaRequired && !captchaToken)}
              onClick={() => {
                void form.validateFields(['phoneNumber']).then(({ phoneNumber }) => {
                  sendCode.mutate({ phoneNumber, captchaToken })
                })
              }}
            >
              {cooldown > 0 ? `${cooldown}秒后可重发` : '发送验证码'}
            </Button>
          }
        />
      </Form.Item>
      {captcha}
      <Button
        type="primary"
        htmlType="submit"
        block
        loading={submitting}
        disabled={captchaRequired && !captchaToken}
      >
        {'登录'}
      </Button>
    </Form>
  )
}
