import { BellOutlined } from '@ant-design/icons'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import { createConnectQueryKey, useMutation, useQuery } from '@connectrpc/connect-query'
import {
  getUnreadCount,
  listNotifications,
  markAllNotificationsRead,
  type Notification as NotificationMessage,
} from '@moonbase/api-client'
import { useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Badge, Button, Dropdown, Empty, Spin } from 'antd'
import { useState } from 'react'

function BellItem({ n, onNavigate }: { n: NotificationMessage; onNavigate: () => void }) {
  const inner = (
    <div className={`px-4 py-2.5 ${n.readAt ? '' : 'bg-(--ant-color-primary-bg)'}`}>
      <div className="truncate text-sm font-medium">{n.title}</div>
      {n.body ? (
        <div className="mt-0.5 line-clamp-2 text-xs text-(--ant-color-text-secondary)">
          {n.body}
        </div>
      ) : null}
      <div className="mt-1 text-[11px] text-(--ant-color-text-tertiary)">
        {n.createdAt ? timestampDate(n.createdAt).toLocaleString() : ''}
      </div>
    </div>
  )
  return n.link ? (
    <Link to={n.link} onClick={onNavigate} className="block hover:bg-black/5 dark:hover:bg-white/5">
      {inner}
    </Link>
  ) : (
    inner
  )
}

export function NotificationBell() {
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)

  const { data: countData } = useQuery(getUnreadCount, {}, { refetchInterval: 30_000 })
  const unread = Number(countData?.unread ?? 0)

  const { data: listData, isLoading } = useQuery(
    listNotifications,
    { page: 0, pageSize: 6, unreadOnly: false },
    { enabled: open },
  )

  const invalidate = () => {
    void queryClient.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: getUnreadCount, cardinality: 'finite' }),
    })
    void queryClient.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listNotifications, cardinality: 'finite' }),
    })
  }

  const markAll = useMutation(markAllNotificationsRead, { onSuccess: invalidate })
  const items = listData?.notifications ?? []

  return (
    <Dropdown
      open={open}
      onOpenChange={setOpen}
      trigger={['click']}
      popupRender={() => (
        <div className="w-80 rounded-lg bg-(--ant-color-bg-elevated) shadow-(--ant-box-shadow-secondary)">
          <div className="flex items-center justify-between border-b border-(--ant-color-split) px-4 py-2.5">
            <span className="text-sm font-medium">{'消息中心'}</span>
            {unread > 0 ? (
              <Button
                type="link"
                size="small"
                loading={markAll.isPending}
                onClick={() => markAll.mutate({})}
              >
                {'全部已读'}
              </Button>
            ) : null}
          </div>
          <div className="max-h-96 overflow-y-auto">
            {isLoading ? (
              <div className="flex justify-center py-8">
                <Spin />
              </div>
            ) : items.length === 0 ? (
              <Empty
                className="py-8"
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description={'暂无消息'}
              />
            ) : (
              items.map((n) => <BellItem key={n.id} n={n} onNavigate={() => setOpen(false)} />)
            )}
          </div>
          <div className="border-t border-(--ant-color-split) px-4 py-2 text-center">
            <Link to="/notifications" onClick={() => setOpen(false)} className="text-sm">
              {'查看全部'}
            </Link>
          </div>
        </div>
      )}
    >
      <button
        type="button"
        aria-label={'消息'}
        className="flex cursor-pointer items-center rounded-lg border-0 bg-transparent px-2 py-1.5 transition-colors hover:bg-black/5 dark:hover:bg-white/10"
      >
        <Badge count={unread} size="small" overflowCount={99}>
          <BellOutlined className="text-lg" />
        </Badge>
      </button>
    </Dropdown>
  )
}
