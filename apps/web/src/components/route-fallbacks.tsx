import { useRouter } from '@tanstack/react-router'
import { Button, Card, Result, Skeleton } from 'antd'
import { ForbiddenError } from '#lib/session'

export function RouteError({ error }: { error: Error }) {
  const router = useRouter()
  if (error instanceof ForbiddenError) {
    return <Result status="403" title="403" subTitle={'您没有权限访问此页面'} />
  }
  return (
    <Result
      status="error"
      title={'出错了'}
      subTitle={'请求失败，请稍后重试'}
      extra={
        <Button type="primary" onClick={() => router.invalidate()}>
          {'重试'}
        </Button>
      }
    />
  )
}

export function RouteNotFound() {
  return <Result status="404" title="404" subTitle={'页面不存在'} />
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
