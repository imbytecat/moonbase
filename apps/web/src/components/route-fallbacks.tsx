import { useRouter } from '@tanstack/react-router'
import { Button, Card, Result, Skeleton } from 'antd'
import { ForbiddenError } from '#lib/session'
import { m } from '#paraglide/messages.js'

export function RouteError({ error }: { error: Error }) {
  const router = useRouter()
  if (error instanceof ForbiddenError) {
    return <Result status="403" title="403" subTitle={m.error_forbidden()} />
  }
  return (
    <Result
      status="error"
      title={m.error_somethingWrong()}
      subTitle={m.error_generic()}
      extra={
        <Button type="primary" onClick={() => router.invalidate()}>
          {m.common_retry()}
        </Button>
      }
    />
  )
}

export function RouteNotFound() {
  return <Result status="404" title="404" subTitle={m.error_notFoundTitle()} />
}

export function RoutePending() {
  return (
    <div className="mx-auto max-w-6xl space-y-4">
      <Card>
        <Skeleton active title={{ width: 180 }} paragraph={{ rows: 1, width: 320 }} />
      </Card>
      <Card>
        <Skeleton active title={false} paragraph={{ rows: 4 }} />
      </Card>
      <Card>
        <Skeleton active title={false} paragraph={{ rows: 6 }} />
      </Card>
    </div>
  )
}
