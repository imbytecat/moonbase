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
import { m } from '#paraglide/messages.js'

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
        label={m.auth_identifier()}
        rules={[{ required: true, message: m.auth_identifierRule() }]}
      >
        <Input autoComplete="username" />
      </Form.Item>
      <Form.Item
        name="password"
        label={m.auth_password()}
        rules={[{ required: true, message: m.auth_passwordRequired() }]}
      >
        <Input.Password autoComplete="current-password" />
      </Form.Item>
      {authConfig?.emailEnabled ? (
        <div className="mb-3 text-end text-sm">
          <Link to="/forgot-password">{m.auth_forgotPassword()}</Link>
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
        {m.auth_signIn()}
      </Button>
    </Form>
  )

  return (
    <AuthShell subtitle={m.auth_signInTitle()}>
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
          <Alert type="info" title={m.auth_totpPrompt()} className="mb-4" showIcon />
          <Form.Item
            name="code"
            label={m.auth_totpCode()}
            rules={[{ required: true, min: 6, message: m.auth_codeRule() }]}
          >
            <Input autoComplete="one-time-code" autoFocus maxLength={32} />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={totpLogin.isPending}>
            {m.auth_signIn()}
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
            {m.auth_backToSignIn()}
          </Button>
        </Form>
      ) : (
        <>
          {authConfig?.smsEnabled ? (
            <Tabs
              centered
              items={[
                { key: 'password', label: m.auth_passwordLogin(), children: passwordForm },
                {
                  key: 'sms',
                  label: m.auth_smsLogin(),
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
                {m.auth_oauthDivider()}
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
              <Typography.Text type="secondary">{m.auth_noAccount()} </Typography.Text>
              <Link to="/register">{m.auth_createOne()}</Link>
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
      message.success(m.auth_codeSent())
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
              disabled={cooldown > 0 || (captchaRequired && !captchaToken)}
              onClick={() => {
                void form.validateFields(['phoneNumber']).then(({ phoneNumber }) => {
                  sendCode.mutate({ phoneNumber, captchaToken })
                })
              }}
            >
              {cooldown > 0 ? m.auth_resendIn({ seconds: cooldown }) : m.auth_sendCode()}
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
        {m.auth_signIn()}
      </Button>
    </Form>
  )
}
