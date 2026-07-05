import { timestampDate } from '@bufbuild/protobuf/wkt'
import { useQuery } from '@connectrpc/connect-query'
import { type AuditLog, listAuditLogs, Permission } from '@moonbase/api-client'
import { createFileRoute } from '@tanstack/react-router'
import { Card, Input, Select, Table, Tag, Tooltip } from 'antd'
import { useState } from 'react'
import { requirePermission } from '#lib/session'
import { m } from '#paraglide/messages.js'

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
    <Card title={m.auditPage_title()}>
      <div className="mb-4 flex flex-wrap gap-2">
        <Select
          allowClear
          className="min-w-40"
          placeholder={m.auditPage_filterDomain()}
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
          placeholder={m.auditPage_filterActor()}
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
            title: m.auditPage_time(),
            key: 'time',
            width: 180,
            render: (_, log) =>
              log.createdAt ? timestampDate(log.createdAt).toLocaleString() : '',
          },
          {
            title: m.auditPage_actor(),
            key: 'actor',
            render: (_, log) =>
              log.actorName || (log.actorId ? <code className="text-xs">{log.actorId}</code> : '—'),
          },
          {
            title: m.auditPage_action(),
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
            title: m.auditPage_resource(),
            dataIndex: 'resourceId',
            render: (v: string) => (v ? <code className="text-xs">{v}</code> : '—'),
          },
          {
            title: m.auditPage_result(),
            dataIndex: 'result',
            width: 120,
            render: (v: string) =>
              v === 'ok' ? (
                <Tag color="success">{m.auditPage_resultOk()}</Tag>
              ) : (
                <Tag color="error">{v}</Tag>
              ),
          },
          { title: 'IP', dataIndex: 'ip', width: 140 },
        ]}
      />
    </Card>
  )
}
