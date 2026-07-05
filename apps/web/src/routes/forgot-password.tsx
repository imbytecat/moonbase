import { createQueryOptions, useMutation, useQuery } from '@connectrpc/connect-query'
import { getAuthConfig, requestPasswordReset } from '@moonbase/api-client'
import { createFileRoute, Link, redirect } from '@tanstack/react-router'
import { Alert, Button, Form, Input } from 'antd'
import { useState } from 'react'
import { AuthShell } from '#components/auth-shell'
import { CaptchaWidget } from '#components/captcha-widget'
import { humanizeError } from '#lib/errors'
import { m } from '#paraglide/messages.js'

export const Route = createFileRoute('/forgot-password')({
  beforeLoad: async ({ context: { queryClient, transport } }) => {
    const config = await queryClient.ensureQueryData(
      createQueryOptions(getAuthConfig, undefined, { transport }),
    )
    if (!config.emailEnabled) throw redirect({ to: '/login' })
  },
  component: ForgotPasswordPage,
})

function ForgotPasswordPage() {
  const { data: authConfig } = useQuery(getAuthConfig)
  const [error, setError] = useState<string>()
  const [sent, setSent] = useState(false)
  const [captchaToken, setCaptchaToken] = useState('')

  const captchaRequired = Boolean(authConfig?.captchaProvider)

  const requestMutation = useMutation(requestPasswordReset, {
    onSuccess: () => setSent(true),
    onError: (err) => {
      setCaptchaToken('')
      setError(humanizeError(err))
    },
  })

  return (
    <AuthShell subtitle={m.auth_forgotHint()}>
      {error ? <Alert type="error" title={error} className="mb-4" showIcon /> : null}
      {sent ? (
        <Alert type="success" title={m.auth_resetLinkSent()} className="mb-4" showIcon />
      ) : (
        <Form
          layout="vertical"
          requiredMark={false}
          disabled={requestMutation.isPending}
          onFinish={(values: { email: string }) => {
            setError(undefined)
            requestMutation.mutate({ ...values, captchaToken })
          }}
        >
          <Form.Item
            name="email"
            label={m.auth_email()}
            rules={[{ required: true, type: 'email', message: m.auth_emailRule() }]}
          >
            <Input autoComplete="email" />
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
            loading={requestMutation.isPending}
            disabled={captchaRequired && !captchaToken}
          >
            {m.auth_sendResetLink()}
          </Button>
        </Form>
      )}

      <div className="mt-4 text-center text-sm">
        <Link to="/login">{m.auth_backToSignIn()}</Link>
      </div>
    </AuthShell>
  )
}
