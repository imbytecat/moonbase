import { useMutation } from '@connectrpc/connect-query'
import { resetPassword } from '@moonbase/api-client'
import { createFileRoute, Link, useRouter } from '@tanstack/react-router'
import { Alert, App, Button, Form, Input } from 'antd'
import { useState } from 'react'
import { AuthShell } from '#components/auth-shell'
import { humanizeError } from '#lib/errors'

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
      message.success('密码已重置，其他设备已全部退出，请用新密码登录')
      await router.navigate({ to: '/login' })
    },
    onError: (err) => setError(humanizeError(err)),
  })

  return (
    <AuthShell subtitle={'设置新密码'}>
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
            label={'新密码'}
            rules={[{ required: true, min: 8, message: '密码至少 8 位' }]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={resetMutation.isPending}>
            {'重置密码'}
          </Button>
        </Form>
      ) : (
        <Alert type="error" title={'链接无效或已过期'} showIcon />
      )}

      <div className="mt-4 text-center text-sm">
        <Link to="/login">{'返回登录'}</Link>
      </div>
    </AuthShell>
  )
}
