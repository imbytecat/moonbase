import { timestampDate } from '@bufbuild/protobuf/wkt'
import { useQuery } from '@connectrpc/connect-query'
import { type AuditLog, listAuditLogs, Permission } from '@moonbase/api-client'
import { createFileRoute } from '@tanstack/react-router'
import { Card, Input, Select, Table, Tag, Tooltip } from 'antd'
import { useState } from 'react'
import { requirePermission } from '#lib/session'

export const Route = createFileRoute('/_authed/audit')({
  beforeLoad: ({ context: { queryClient, transport } }) =>
    requirePermission(queryClient, transport, Permission.AUDIT_READ),
  component: AuditPage,
})

const PAGE_SIZE = 20

const DOMAIN_OPTIONS = ['auth', 'user', 'role', 'settings', 'system', 'storage', 'workflow']

function actionMethod(action: string): string {
  return action.slice(action.lastIndexOf('/') + 1)
}

function AuditPage() {
  const [page, setPage] = useState(0)
  const [domain, setDomain] = useState('')
  const [actorId, setActorId] = useState('')

  const { data, isFetching } = useQuery(listAuditLogs, {
    page,
    pageSize: PAGE_SIZE,
    domain,
    actorId,
  })

  return (
    <Card title={'审计日志'}>
      <div className="mb-4 flex flex-wrap gap-2">
        <Select
          allowClear
          className="min-w-40"
          placeholder={'按模块筛选'}
          value={domain || undefined}
          onChange={(v) => {
            setDomain(v ?? '')
            setPage(0)
          }}
          options={DOMAIN_OPTIONS.map((d) => ({ value: d, label: d }))}
        />
        <Input.Search
          allowClear
          className="max-w-80"
          placeholder={'按操作人 ID 筛选'}
          onSearch={(v) => {
            setActorId(v.trim())
            setPage(0)
          }}
        />
      </div>
      <Table<AuditLog>
        rowKey="id"
        loading={isFetching}
        dataSource={data?.logs ?? []}
        scroll={{ x: 'max-content' }}
        pagination={{
          current: page + 1,
          pageSize: PAGE_SIZE,
          total: Number(data?.total ?? 0),
          showSizeChanger: false,
          onChange: (p) => setPage(p - 1),
        }}
        columns={[
          {
            title: '时间',
            key: 'time',
            width: 180,
            render: (_, log) =>
              log.createdAt ? timestampDate(log.createdAt).toLocaleString() : '',
          },
          {
            title: '操作人',
            key: 'actor',
            render: (_, log) =>
              log.actorName || (log.actorId ? <code className="text-xs">{log.actorId}</code> : '—'),
          },
          {
            title: '操作',
            key: 'action',
            render: (_, log) => (
              <Tooltip title={log.action}>
                <span>
                  <Tag>{log.domain}</Tag>
                  <code className="text-xs">{actionMethod(log.action)}</code>
                </span>
              </Tooltip>
            ),
          },
          {
            title: '资源',
            dataIndex: 'resourceId',
            render: (v: string) => (v ? <code className="text-xs">{v}</code> : '—'),
          },
          {
            title: '结果',
            dataIndex: 'result',
            width: 120,
            render: (v: string) =>
              v === 'ok' ? <Tag color="success">{'成功'}</Tag> : <Tag color="error">{v}</Tag>,
          },
          { title: 'IP', dataIndex: 'ip', width: 140 },
        ]}
      />
    </Card>
  )
}
