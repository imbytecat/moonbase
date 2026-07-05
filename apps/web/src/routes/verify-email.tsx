import { useMutation } from '@connectrpc/connect-query'
import { verifyEmail } from '@moonbase/api-client'
import { createFileRoute, Link } from '@tanstack/react-router'
import { Button, Result, Spin } from 'antd'
import { useEffect } from 'react'
import { AuthShell } from '#components/auth-shell'
import { humanizeError } from '#lib/errors'
import { m } from '#paraglide/messages.js'

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
        title={m.auth_verifyFailed()}
        subTitle={verifyMutation.error ? humanizeError(verifyMutation.error) : undefined}
        extra={
          <Link to="/login">
            <Button type="primary">{m.auth_backToSignIn()}</Button>
          </Link>
        }
      />
    )
  } else if (verifyMutation.isSuccess) {
    content = (
      <Result
        status="success"
        title={m.auth_verifySuccess()}
        extra={
          <Link to="/">
            <Button type="primary">{m.auth_goToApp()}</Button>
          </Link>
        }
      />
    )
  } else {
    content = (
      <div className="flex flex-col items-center gap-4 p-8">
        <Spin size="large" />
        <span className="text-(--ant-color-text-secondary)">{m.auth_verifying()}</span>
      </div>
    )
  }

  return <AuthShell>{content}</AuthShell>
}
