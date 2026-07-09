import { useMutation } from '@connectrpc/connect-query'
import { verifyEmail } from '@moonbase/api-client'
import { createFileRoute, Link } from '@tanstack/react-router'
import { Button, Result, Spin } from 'antd'
import { useEffect } from 'react'
import { AuthShell } from '#components/auth-shell'
import { humanizeError } from '#lib/errors'

export interface VerifySearch {
  token?: string
}

export const Route = createFileRoute('/verify-email')({
  validateSearch: (search: Record<string, unknown>): VerifySearch => ({
    token: typeof search.token === 'string' ? search.token : undefined,
  }),
  component: VerifyEmailPage,
})

function VerifyEmailPage() {
  const { token } = Route.useSearch()
  const verifyMutation = useMutation(verifyEmail)
  const { mutate: doVerify } = verifyMutation

  useEffect(() => {
    if (token) doVerify({ token })
  }, [token, doVerify])

  let content: React.ReactNode
  if (!token || verifyMutation.isError) {
    content = (
      <Result
        status="error"
        title={'链接无效或已过期'}
        subTitle={verifyMutation.error ? humanizeError(verifyMutation.error) : undefined}
        extra={
          <Link to="/login">
            <Button type="primary">{'返回登录'}</Button>
          </Link>
        }
      />
    )
  } else if (verifyMutation.isSuccess) {
    content = (
      <Result
        status="success"
        title={'邮箱验证成功'}
        extra={
          <Link to="/">
            <Button type="primary">{'进入应用'}</Button>
          </Link>
        }
      />
    )
  } else {
    content = (
      <div className="flex flex-col items-center gap-4 p-8">
        <Spin size="large" />
        <span className="text-(--ant-color-text-secondary)">{'正在验证…'}</span>
      </div>
    )
  }

  return <AuthShell>{content}</AuthShell>
}
