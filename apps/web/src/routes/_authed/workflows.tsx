import { PlayCircleOutlined, ReloadOutlined } from '@ant-design/icons'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import {
  createConnectQueryKey,
  createQueryOptions,
  useMutation,
  useQuery,
  useSuspenseQuery,
} from '@connectrpc/connect-query'
import {
  cancelWorkflowRun,
  getMe,
  getWorkflowRun,
  listWorkflowRuns,
  Permission,
  resumeWorkflowRun,
  triggerDemoWorkflow,
  type WorkflowRun,
} from '@moonbase/api-client'
import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { App, Button, Card, Drawer, Empty, Table, Tag, Typography } from 'antd'
import { useState } from 'react'
import { WorkflowDag } from '#components/workflow-dag'
import { humanizeError } from '#lib/errors'
import { hasPermission, requirePermission } from '#lib/session'

export const Route = createFileRoute('/_authed/workflows')({
  beforeLoad: ({ context: { queryClient, transport } }) =>
    requirePermission(queryClient, transport, Permission.WORKFLOW_READ),
  loader: ({ context: { queryClient, transport } }) =>
    queryClient.ensureQueryData(createQueryOptions(listWorkflowRuns, undefined, { transport })),
  component: WorkflowsPage,
})

const STATUS_COLORS: Record<string, string> = {
  SUCCESS: 'green',
  ERROR: 'red',
  CANCELLED: 'default',
  PENDING: 'blue',
  ENQUEUED: 'gold',
}

// Runs transition while the page is open, so poll: cheap list query, only
// while the tab is visible (TanStack Query pauses hidden-tab intervals).
const POLL_MS = 3000

function WorkflowsPage() {
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const { data } = useSuspenseQuery(listWorkflowRuns, undefined, {
    refetchInterval: POLL_MS,
  })
  const { data: session } = useSuspenseQuery(getMe)
  const [selectedId, setSelectedId] = useState<string>()

  const canWrite = hasPermission(session?.user, Permission.WORKFLOW_WRITE)

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listWorkflowRuns, cardinality: 'finite' }),
    })

  const triggerMutation = useMutation(triggerDemoWorkflow, {
    onSuccess: (res) => {
      void invalidate()
      setSelectedId(res.id)
      message.success('工作流已启动')
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  return (
    <div className="mx-auto max-w-5xl">
      <Card
        title={'工作流'}
        extra={
          canWrite ? (
            <Button
              type="primary"
              icon={<PlayCircleOutlined />}
              loading={triggerMutation.isPending}
              onClick={() => triggerMutation.mutate({ name: 'demo' })}
            >
              {'运行示例工作流'}
            </Button>
          ) : null
        }
      >
        {data.runs.length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={'暂无工作流运行记录'} />
        ) : (
          <Table<WorkflowRun>
            rowKey="id"
            dataSource={data.runs}
            pagination={false}
            scroll={{ x: 'max-content' }}
            size="middle"
            onRow={(run) => ({
              onClick: () => setSelectedId(run.id),
              className: 'cursor-pointer',
            })}
            columns={[
              {
                title: '名称',
                dataIndex: 'name',
                render: (name: string) => (
                  <Typography.Text strong className="text-[13px]">
                    {name.split('.').at(-1)}
                  </Typography.Text>
                ),
              },
              {
                title: '状态',
                dataIndex: 'status',
                width: 130,
                render: (status: string) => (
                  <Tag color={STATUS_COLORS[status] ?? 'default'}>{status}</Tag>
                ),
              },
              {
                title: '创建时间',
                dataIndex: 'createdAt',
                width: 200,
                render: (_: unknown, run) =>
                  run.createdAt ? timestampDate(run.createdAt).toLocaleString() : '—',
              },
              {
                title: '尝试',
                dataIndex: 'attempts',
                width: 90,
              },
            ]}
          />
        )}
      </Card>

      <RunDrawer
        id={selectedId}
        canWrite={canWrite}
        onClose={() => setSelectedId(undefined)}
        onChanged={() => void invalidate()}
      />
    </div>
  )
}

function RunDrawer({
  id,
  canWrite,
  onClose,
  onChanged,
}: {
  id: string | undefined
  canWrite: boolean
  onClose: () => void
  onChanged: () => void
}) {
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const { data } = useQuery(getWorkflowRun, id ? { id } : undefined, {
    enabled: Boolean(id),
    refetchInterval: POLL_MS,
  })

  const invalidateRun = () => {
    onChanged()
    if (id) {
      void queryClient.invalidateQueries({
        queryKey: createConnectQueryKey({
          schema: getWorkflowRun,
          input: { id },
          cardinality: 'finite',
        }),
      })
    }
  }

  const cancelMutation = useMutation(cancelWorkflowRun, {
    onSuccess: () => {
      invalidateRun()
      message.success('已取消')
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const resumeMutation = useMutation(resumeWorkflowRun, {
    onSuccess: () => {
      invalidateRun()
      message.success('已恢复')
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const run = data?.run
  const running = run?.status === 'PENDING' || run?.status === 'ENQUEUED'

  return (
    <Drawer
      title={run ? run.name.split('.').at(-1) : ''}
      open={Boolean(id)}
      onClose={onClose}
      size="min(720px, 100vw)"
      destroyOnHidden
      extra={
        canWrite && run ? (
          <div className="flex gap-2">
            {running ? (
              <Button
                danger
                size="small"
                loading={cancelMutation.isPending}
                onClick={() => cancelMutation.mutate({ id: run.id })}
              >
                {'取消'}
              </Button>
            ) : null}
            {run.status === 'CANCELLED' || run.status === 'ERROR' ? (
              <Button
                size="small"
                icon={<ReloadOutlined />}
                loading={resumeMutation.isPending}
                onClick={() => resumeMutation.mutate({ id: run.id })}
              >
                {'恢复'}
              </Button>
            ) : null}
          </div>
        ) : null
      }
    >
      {run ? (
        <div className="space-y-4">
          <div className="flex flex-wrap items-center gap-2 text-xs text-(--ant-color-text-tertiary)">
            <Tag color={STATUS_COLORS[run.status] ?? 'default'}>{run.status}</Tag>
            <span>{run.id}</span>
          </div>

          <WorkflowDag
            workflowName={run.name.split('.').at(-1) ?? run.name}
            steps={data?.steps ?? []}
          />

          {run.output ? (
            <div>
              <Typography.Text strong className="text-xs">
                {'输出'}
              </Typography.Text>
              <pre className="mt-1 overflow-auto rounded-lg bg-(--ant-color-fill-quaternary) p-3 text-xs">
                {run.output}
              </pre>
            </div>
          ) : null}
          {run.error ? (
            <div>
              <Typography.Text strong type="danger" className="text-xs">
                {'错误'}
              </Typography.Text>
              <pre className="mt-1 overflow-auto rounded-lg bg-(--ant-color-error-bg) p-3 text-xs">
                {run.error}
              </pre>
            </div>
          ) : null}
        </div>
      ) : null}
    </Drawer>
  )
}
