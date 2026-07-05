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
import { m } from '#paraglide/messages.js'

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
    onSuccess: () => message.success(m.auth_codeSent()),
    onError: (err) => setError(humanizeError(err)),
  })
  const sendPhoneCode = useMutation(sendPhoneRegisterCode, {
    onSuccess: () => message.success(m.auth_codeSent()),
    onError: (err) => setError(humanizeError(err)),
  })

  return (
    <AuthShell subtitle={m.auth_oauthCompleteTitle()}>
      {error ? <Alert type="error" title={error} className="mb-4" showIcon /> : null}
      <Alert type="info" title={m.auth_oauthCompleteHint()} className="mb-4" showIcon />

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
        <Form.Item
          name="name"
          label={m.auth_name()}
          rules={[{ required: true, message: m.auth_nameRule() }]}
        >
          <Input autoComplete="name" />
        </Form.Item>
        {collectUsername ? (
          <Form.Item
            name="username"
            label={m.auth_username()}
            rules={[
              {
                required: true,
                pattern: /^[a-zA-Z][a-zA-Z0-9._-]{2,31}$/,
                message: m.auth_usernameRule(),
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
              label={m.auth_email()}
              rules={[{ required: true, type: 'email', message: m.auth_emailRule() }]}
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
            <Form.Item name="phone" label={m.auth_phone()} rules={[phoneRule()]}>
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
          label={m.auth_password()}
          rules={[{ required: true, min: 8, message: m.auth_passwordRule() }]}
        >
          <Input.Password autoComplete="new-password" />
        </Form.Item>
        <Button type="primary" htmlType="submit" block loading={completeMutation.isPending}>
          {m.auth_register()}
        </Button>
      </Form>

      <div className="mt-4 text-center text-sm">
        <Link to="/login">{m.auth_backToSignIn()}</Link>
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
            loading={sending}
            disabled={cooldown > 0}
            onClick={sendCode}
          >
            {cooldown > 0 ? m.auth_resendIn({ seconds: cooldown }) : m.auth_sendCode()}
          </Button>
        }
      />
    </Form.Item>
  )
}
