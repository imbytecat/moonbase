import { createQueryOptions, useMutation, useQuery } from '@connectrpc/connect-query'
import {
  getAuthConfig,
  register,
  sendEmailRegisterCode,
  sendPhoneRegisterCode,
} from '@moonbase/api-client'
import { createFileRoute, Link, redirect, useRouter } from '@tanstack/react-router'
import { Alert, App, Button, Form, type FormInstance, Input } from 'antd'
import { useState } from 'react'
import { AuthShell } from '#components/auth-shell'
import { CaptchaWidget } from '#components/captcha-widget'
import { PhoneInput, phoneRule } from '#components/phone-input'
import { humanizeError } from '#lib/errors'

export const Route = createFileRoute('/register')({
  beforeLoad: async ({ context: { queryClient, transport } }) => {
    const config = await queryClient.ensureQueryData(
      createQueryOptions(getAuthConfig, undefined, { transport }),
    )
    if (!config.registrationEnabled) throw redirect({ to: '/login' })
  },
  component: RegisterPage,
})

interface RegisterFormValues {
  name: string
  username?: string
  email?: string
  emailCode?: string
  phone?: string
  phoneCode?: string
  password: string
}

function RegisterPage() {
  const router = useRouter()
  const { message } = App.useApp()
  const { data: authConfig } = useQuery(getAuthConfig)
  const [error, setError] = useState<string>()
  const [captchaToken, setCaptchaToken] = useState('')
  const [form] = Form.useForm<RegisterFormValues>()

  const captchaRequired = Boolean(authConfig?.captchaProvider)
  const identifiers = authConfig?.signupIdentifiers ?? ['username']
  const collectUsername = identifiers.includes('username')
  const collectEmail = identifiers.includes('email')
  const collectPhone = identifiers.includes('phone')

  const registerMutation = useMutation(register, {
    onSuccess: async () => {
      message.success('账号创建成功，请登录')
      await router.navigate({ to: '/login' })
    },
    onError: (err) => {
      setCaptchaToken('')
      setError(humanizeError(err))
    },
  })

  const sendEmailCode = useMutation(sendEmailRegisterCode, {
    onSuccess: () => message.success('验证码已发送'),
    onError: (err) => setError(humanizeError(err)),
  })
  const sendPhoneCode = useMutation(sendPhoneRegisterCode, {
    onSuccess: () => message.success('验证码已发送'),
    onError: (err) => setError(humanizeError(err)),
  })

  return (
    <AuthShell subtitle={'创建账号'}>
      {error ? <Alert type="error" title={error} className="mb-4" showIcon /> : null}

      <Form
        form={form}
        layout="vertical"
        requiredMark={false}
        disabled={registerMutation.isPending}
        onFinish={(values: RegisterFormValues) => {
          setError(undefined)
          registerMutation.mutate({ ...values, captchaToken })
        }}
      >
        <Form.Item name="name" label={'姓名'} rules={[{ required: true, message: '请输入姓名' }]}>
          <Input autoComplete="name" />
        </Form.Item>
        {collectUsername ? (
          <Form.Item
            name="username"
            label={'用户名'}
            rules={[
              {
                required: true,
                pattern: /^[a-zA-Z][a-zA-Z0-9._-]{2,31}$/,
                message: '3-32 位，字母开头，可含字母、数字、. _ -',
              },
            ]}
          >
            <Input autoComplete="username" />
          </Form.Item>
        ) : null}
        {collectEmail ? (
          <>
            <Form.Item
              name="email"
              label={'邮箱'}
              rules={[{ required: true, type: 'email', message: '请输入有效的邮箱地址' }]}
            >
              <Input autoComplete="email" />
            </Form.Item>
            <CodeItem
              form={form}
              name="emailCode"
              sourceField="email"
              captchaToken={captchaToken}
              captchaRequired={captchaRequired}
              sending={sendEmailCode.isPending}
              onSend={(email) => sendEmailCode.mutate({ email, captchaToken })}
            />
          </>
        ) : null}
        {collectPhone ? (
          <>
            <Form.Item name="phone" label={'手机号'} rules={[phoneRule()]}>
              <PhoneInput allowedRegions={authConfig?.allowedPhoneRegions ?? []} />
            </Form.Item>
            <CodeItem
              form={form}
              name="phoneCode"
              sourceField="phone"
              captchaToken={captchaToken}
              captchaRequired={captchaRequired}
              sending={sendPhoneCode.isPending}
              onSend={(phoneNumber) => sendPhoneCode.mutate({ phoneNumber, captchaToken })}
            />
          </>
        ) : null}
        <Form.Item
          name="password"
          label={'密码'}
          rules={[{ required: true, min: 8, message: '密码至少 8 位' }]}
        >
          <Input.Password autoComplete="new-password" />
        </Form.Item>
        {captchaRequired ? (
          <CaptchaWidget
            provider={authConfig?.captchaProvider ?? ''}
            siteKey={authConfig?.captchaSiteKey ?? ''}
            onToken={setCaptchaToken}
          />
        ) : null}
        <Button
          type="primary"
          htmlType="submit"
          block
          loading={registerMutation.isPending}
          disabled={captchaRequired && !captchaToken}
        >
          {'注册'}
        </Button>
      </Form>

      <div className="mt-4 text-center text-sm">
        <Link to="/login">{'返回登录'}</Link>
      </div>
    </AuthShell>
  )
}

function CodeItem({
  form,
  name,
  sourceField,
  captchaToken,
  captchaRequired,
  sending,
  onSend,
}: {
  form: FormInstance<RegisterFormValues>
  name: 'emailCode' | 'phoneCode'
  sourceField: 'email' | 'phone'
  captchaToken: string
  captchaRequired: boolean
  sending: boolean
  onSend: (source: string) => void
}) {
  const [cooldown, setCooldown] = useState(0)

  const sendCode = () => {
    void form.validateFields([sourceField]).then((values) => {
      const source = values[sourceField]
      if (!source) return
      onSend(source)
      setCooldown(60)
      const timer = setInterval(() => {
        setCooldown((s) => {
          if (s <= 1) clearInterval(timer)
          return s - 1
        })
      }, 1000)
    })
  }

  return (
    <Form.Item
      name={name}
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
            loading={sending}
            disabled={cooldown > 0 || (captchaRequired && !captchaToken)}
            onClick={sendCode}
          >
            {cooldown > 0 ? `${cooldown}秒后可重发` : '发送验证码'}
          </Button>
        }
      />
    </Form.Item>
  )
}
