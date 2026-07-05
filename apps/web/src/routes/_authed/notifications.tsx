import { timestampDate } from '@bufbuild/protobuf/wkt'
import { createConnectQueryKey, useMutation, useQuery } from '@connectrpc/connect-query'
import {
  deleteNotification,
  getUnreadCount,
  listNotifications,
  markAllNotificationsRead,
  markNotificationsRead,
  type Notification as NotificationMessage,
} from '@moonbase/api-client'
import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute, Link } from '@tanstack/react-router'
import { App, Button, Card, Empty, List, Pagination, Popconfirm, Segmented, Tag } from 'antd'
import { type ReactNode, useState } from 'react'
import { humanizeError } from '#lib/errors'
import { notificationCategory } from '#lib/notifications'
import { m } from '#paraglide/messages.js'

export const Route = createFileRoute('/_authed/notifications')({
  component: NotificationsPage,
})

const PAGE_SIZE = 20

function NotificationsPage() {
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(0)
  const [unreadOnly, setUnreadOnly] = useState(false)

  const { data, isFetching } = useQuery(listNotifications, {
    page,
    pageSize: PAGE_SIZE,
    unreadOnly,
  })

  const invalidate = () => {
    void queryClient.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listNotifications, cardinality: 'finite' }),
    })
    void queryClient.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: getUnreadCount, cardinality: 'finite' }),
    })
  }

  const markRead = useMutation(markNotificationsRead, {
    onSuccess: invalidate,
    onError: (err) => message.error(humanizeError(err)),
  })
  const markAll = useMutation(markAllNotificationsRead, {
    onSuccess: () => {
      invalidate()
      message.success(m.notifications_allRead())
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const remove = useMutation(deleteNotification, {
    onSuccess: invalidate,
    onError: (err) => message.error(humanizeError(err)),
  })

  const items = data?.notifications ?? []

  const renderItem = (n: NotificationMessage) => {
    const cat = notificationCategory(n.category)
    const actions: ReactNode[] = []
    if (!n.readAt) {
      actions.push(
        <Button
          key="read"
          type="link"
          size="small"
          onClick={() => markRead.mutate({ ids: [n.id] })}
        >
          {m.notifications_markRead()}
        </Button>,
      )
    }
    if (n.link) {
      actions.push(
        <Link key="view" to={n.link}>
          {m.notifications_view()}
        </Link>,
      )
    }
    actions.push(
      <Popconfirm
        key="delete"
        title={m.notifications_delete()}
        onConfirm={() => remove.mutate({ id: n.id })}
      >
        <Button type="link" size="small" danger>
          {m.notifications_delete()}
        </Button>
      </Popconfirm>,
    )

    return (
      <List.Item className={n.readAt ? '' : 'bg-(--ant-color-primary-bg)'} actions={actions}>
        <List.Item.Meta
          title={
            <span className="flex items-center gap-2">
              {n.readAt ? null : <span className="size-2 rounded-full bg-(--ant-color-primary)" />}
              <span>{n.title}</span>
              <Tag color={cat.color}>{cat.label}</Tag>
            </span>
          }
          description={
            <>
              {n.body ? <div className="text-(--ant-color-text-secondary)">{n.body}</div> : null}
              <div className="mt-1 text-xs text-(--ant-color-text-tertiary)">
                {n.createdAt ? timestampDate(n.createdAt).toLocaleString() : ''}
              </div>
            </>
          }
        />
      </List.Item>
    )
  }

  return (
    <Card
      title={m.notifications_title()}
      extra={
        <Button
          loading={markAll.isPending}
          disabled={Number(data?.unread ?? 0) === 0}
          onClick={() => markAll.mutate({})}
        >
          {m.notifications_markAllRead()}
        </Button>
      }
    >
      <Segmented
        className="mb-4"
        value={unreadOnly ? 'unread' : 'all'}
        onChange={(v) => {
          setUnreadOnly(v === 'unread')
          setPage(0)
        }}
        options={[
          { label: m.notifications_filterAll(), value: 'all' },
          { label: m.notifications_filterUnread(), value: 'unread' },
        ]}
      />
      {items.length === 0 && !isFetching ? (
        <Empty description={m.notifications_empty()} />
      ) : (
        <List<NotificationMessage>
          loading={isFetching}
          dataSource={items}
          rowKey="id"
          renderItem={renderItem}
        />
      )}
      <div className="mt-4 flex justify-end">
        <Pagination
          current={page + 1}
          pageSize={PAGE_SIZE}
          total={Number(data?.total ?? 0)}
          showSizeChanger={false}
          onChange={(p) => setPage(p - 1)}
        />
      </div>
    </Card>
  )
}
