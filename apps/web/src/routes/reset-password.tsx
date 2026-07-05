import { useMutation } from '@connectrpc/connect-query'
import { resetPassword } from '@moonbase/api-client'
import { createFileRoute, Link, useRouter } from '@tanstack/react-router'
import { Alert, App, Button, Form, Input } from 'antd'
import { useState } from 'react'
import { AuthShell } from '#components/auth-shell'
import { humanizeError } from '#lib/errors'
import { m } from '#paraglide/messages.js'

export interface ResetSearch {
  token?: string
}

export const Route = createFileRoute('/reset-password')({
  validateSearch: (search: Record<string, unknown>): ResetSearch => ({
    token: typeof search.token === 'string' ? search.token : undefined,
  }),
  component: ResetPasswordPage,
})

function ResetPasswordPage() {
  const router = useRouter()
  const { message } = App.useApp()
  const { token } = Route.useSearch()
  const [error, setError] = useState<string>()

  const resetMutation = useMutation(resetPassword, {
    onSuccess: async () => {
      message.success(m.auth_resetSuccess())
      await router.navigate({ to: '/login' })
    },
    onError: (err) => setError(humanizeError(err)),
  })

  return (
    <AuthShell subtitle={m.auth_resetTitle()}>
      {error ? <Alert type="error" title={error} className="mb-4" showIcon /> : null}
      {token ? (
        <Form
          layout="vertical"
          requiredMark={false}
          disabled={resetMutation.isPending}
          onFinish={(values: { newPassword: string }) => {
            setError(undefined)
            resetMutation.mutate({ token, newPassword: values.newPassword })
          }}
        >
          <Form.Item
            name="newPassword"
            label={m.auth_newPassword()}
            rules={[{ required: true, min: 8, message: m.auth_passwordRule() }]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={resetMutation.isPending}>
            {m.auth_resetPassword()}
          </Button>
        </Form>
      ) : (
        <Alert type="error" title={m.auth_verifyFailed()} showIcon />
      )}

      <div className="mt-4 text-center text-sm">
        <Link to="/login">{m.auth_backToSignIn()}</Link>
      </div>
    </AuthShell>
  )
}
