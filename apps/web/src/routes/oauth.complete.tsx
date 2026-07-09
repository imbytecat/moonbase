import { createQueryOptions, useMutation, useQuery } from '@connectrpc/connect-query'
import {
  completeOauthSignup,
  getAuthConfig,
  sendEmailRegisterCode,
  sendPhoneRegisterCode,
} from '@moonbase/api-client'
import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute, Link, redirect, useRouter } from '@tanstack/react-router'
import { Alert, App, Button, Form, type FormInstance, Input } from 'antd'
import { useState } from 'react'
import { AuthShell } from '#components/auth-shell'
import { PhoneInput, phoneRule } from '#components/phone-input'
import { humanizeError } from '#lib/errors'

export interface CompleteSearch {
  ticket: string
  provider: string
  name?: string
}

export const Route = createFileRoute('/oauth/complete')({
  validateSearch: (search: Record<string, unknown>): CompleteSearch => ({
    ticket: typeof search.ticket === 'string' ? search.ticket : '',
    provider: typeof search.provider === 'string' ? search.provider : '',
    name: typeof search.name === 'string' ? search.name : undefined,
  }),
  beforeLoad: async ({ context: { queryClient, transport }, search }) => {
    if (!search.ticket) throw redirect({ to: '/login' })
    const config = await queryClient.ensureQueryData(
      createQueryOptions(getAuthConfig, undefined, { transport }),
    )
    if (!config.registrationEnabled) throw redirect({ to: '/login' })
  },
  component: CompleteOauthSignupPage,
})

interface CompleteFormValues {
  name: string
  username?: string
  email?: string
  emailCode?: string
  phone?: string
  phoneCode?: string
  password: string
}

function CompleteOauthSignupPage() {
  const router = useRouter()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const search = Route.useSearch()
  const { data: authConfig } = useQuery(getAuthConfig)
  const [error, setError] = useState<string>()
  const [form] = Form.useForm<CompleteFormValues>()

  const identifiers = authConfig?.signupIdentifiers ?? ['username']
  const collectUsername = identifiers.includes('username')
  const collectEmail = identifiers.includes('email')
  const collectPhone = identifiers.includes('phone')

  const completeMutation = useMutation(completeOauthSignup, {
    onSuccess: async () => {
      queryClient.clear()
      await router.invalidate()
      await router.navigate({ to: '/' })
    },
    onError: (err) => setError(humanizeError(err)),
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
    <AuthShell subtitle={'完善账号信息'}>
      {error ? <Alert type="error" title={error} className="mb-4" showIcon /> : null}
      <Alert
        type="info"
        title={'微信身份已验证，补充以下信息即可完成注册。'}
        className="mb-4"
        showIcon
      />

      <Form
        form={form}
        layout="vertical"
        requiredMark={false}
        disabled={completeMutation.isPending}
        initialValues={{ name: search.name ?? '' }}
        onFinish={(values: CompleteFormValues) => {
          setError(undefined)
          completeMutation.mutate({ ...values, ticket: search.ticket })
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
              sending={sendEmailCode.isPending}
              onSend={(email) => sendEmailCode.mutate({ email })}
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
              sending={sendPhoneCode.isPending}
              onSend={(phoneNumber) => sendPhoneCode.mutate({ phoneNumber })}
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
        <Button type="primary" htmlType="submit" block loading={completeMutation.isPending}>
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
  sending,
  onSend,
}: {
  form: FormInstance<CompleteFormValues>
  name: 'emailCode' | 'phoneCode'
  sourceField: 'email' | 'phone'
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
            disabled={cooldown > 0}
            onClick={sendCode}
          >
            {cooldown > 0 ? `${cooldown}秒后可重发` : '发送验证码'}
          </Button>
        }
      />
    </Form.Item>
  )
}
