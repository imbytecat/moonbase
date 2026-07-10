import { PlusOutlined } from '@ant-design/icons'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import {
  createConnectQueryKey,
  createQueryOptions,
  useMutation,
  useSuspenseQuery,
} from '@connectrpc/connect-query'
import {
  createDemoCheckout,
  getMe,
  listPaymentOrders,
  type PaymentOrder,
  Permission,
  refundPaymentOrder,
  syncPaymentOrder,
} from '@moonbase/api-client'
import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import {
  App,
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Table,
  Tag,
} from 'antd'
import { useState } from 'react'
import { humanizeError } from '#lib/errors'
import { hasPermission, requirePermission } from '#lib/session'

export const Route = createFileRoute('/_authed/payments')({
  beforeLoad: ({ context: { queryClient, transport } }) =>
    requirePermission(queryClient, transport, Permission.PAYMENT_READ),
  loader: ({ context: { queryClient, transport } }) =>
    queryClient.ensureQueryData(
      createQueryOptions(listPaymentOrders, { page: 0, pageSize: 20 }, { transport }),
    ),
  component: PaymentsPage,
})

const STATUS: Record<string, { label: string; color: string }> = {
  creating: { label: '创建中', color: 'processing' },
  pending: { label: '待支付', color: 'blue' },
  paid: { label: '已支付', color: 'green' },
  failed: { label: '创建失败', color: 'red' },
  closed: { label: '已关闭', color: 'default' },
  refunding: { label: '退款中', color: 'gold' },
  refunded: { label: '已退款', color: 'purple' },
}

const yuan = (cents: bigint) =>
  `¥${(Number(cents) / 100).toLocaleString(undefined, { minimumFractionDigits: 2 })}`

function PaymentsPage() {
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(0)
  const [status, setStatus] = useState('')
  const [demoOpen, setDemoOpen] = useState(false)
  const { data } = useSuspenseQuery(listPaymentOrders, { page, pageSize: 20, status })
  const { data: session } = useSuspenseQuery(getMe)
  const canWrite = hasPermission(session?.user, Permission.PAYMENT_WRITE)

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listPaymentOrders, cardinality: 'finite' }),
    })
  const syncMutation = useMutation(syncPaymentOrder, {
    onSuccess: () => void invalidate(),
    onError: (error) => message.error(humanizeError(error)),
  })
  const refundMutation = useMutation(refundPaymentOrder, {
    onSuccess: () => {
      void invalidate()
      message.success('退款已提交')
    },
    onError: (error) => message.error(humanizeError(error)),
  })

  return (
    <div className="mx-auto max-w-6xl">
      <Card
        title="支付订单"
        extra={
          <div className="flex gap-2">
            <Select
              className="w-32"
              value={status}
              options={[
                { value: '', label: '全部状态' },
                ...Object.entries(STATUS).map(([value, item]) => ({ value, label: item.label })),
              ]}
              onChange={(value) => {
                setPage(0)
                setStatus(value)
              }}
            />
            {canWrite ? (
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setDemoOpen(true)}>
                打开演示收银台
              </Button>
            ) : null}
          </div>
        }
      >
        <Table<PaymentOrder>
          rowKey="id"
          dataSource={data?.orders ?? []}
          pagination={{
            current: page + 1,
            pageSize: 20,
            total: Number(data?.total ?? 0n),
            onChange: (next) => setPage(next - 1),
          }}
          columns={[
            {
              title: '订单',
              render: (_, order) => (
                <div>
                  <div className="font-medium">{order.subject}</div>
                  <div className="text-xs text-(--ant-color-text-tertiary)">{order.outTradeNo}</div>
                </div>
              ),
            },
            { title: '金额', dataIndex: 'amount', render: (amount: bigint) => yuan(amount) },
            {
              title: '实际路径',
              render: (_, order) => `${order.profileName || order.provider} · ${order.productId}`,
            },
            {
              title: '状态',
              dataIndex: 'status',
              render: (value: string) => (
                <Tag color={STATUS[value]?.color}>{STATUS[value]?.label ?? value}</Tag>
              ),
            },
            {
              title: '创建时间',
              dataIndex: 'createdAt',
              render: (value: PaymentOrder['createdAt']) =>
                value ? timestampDate(value).toLocaleString('zh-CN') : '-',
            },
            {
              title: '操作',
              render: (_, order) => (
                <div className="flex gap-1">
                  {canWrite && ['creating', 'pending', 'refunding'].includes(order.status) ? (
                    <Button type="link" onClick={() => syncMutation.mutate({ id: order.id })}>
                      同步
                    </Button>
                  ) : null}
                  {canWrite && order.status === 'paid' ? (
                    <Popconfirm
                      title="确认全额退款？"
                      onConfirm={() =>
                        refundMutation.mutate({ id: order.id, reason: '管理员退款' })
                      }
                    >
                      <Button type="link" danger>
                        退款
                      </Button>
                    </Popconfirm>
                  ) : null}
                </div>
              ),
            },
          ]}
        />
      </Card>
      <DemoCheckoutModal open={demoOpen} onClose={() => setDemoOpen(false)} />
    </div>
  )
}

function DemoCheckoutModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { message } = App.useApp()
  const [form] = Form.useForm<{ subject: string; amount: number }>()
  const createMutation = useMutation(createDemoCheckout, {
    onSuccess: (response) => {
      onClose()
      form.resetFields()
      window.open(response.checkoutUrl, '_blank', 'noopener,noreferrer')
    },
    onError: (error) => message.error(humanizeError(error)),
  })
  return (
    <Modal title="演示收银台" open={open} onCancel={onClose} footer={null}>
      <Form
        form={form}
        layout="vertical"
        initialValues={{ subject: '演示订单', amount: 1 }}
        onFinish={(values) => {
          const reference = crypto.randomUUID()
          createMutation.mutate({
            subject: values.subject,
            amount: BigInt(Math.round(values.amount * 100)),
            businessReference: reference,
            idempotencyKey: reference,
            returnPath: '/payments',
          })
        }}
      >
        <Form.Item name="subject" label="订单标题" rules={[{ required: true }]}>
          <Input maxLength={128} />
        </Form.Item>
        <Form.Item name="amount" label="金额（元）" rules={[{ required: true }]}>
          <InputNumber className="w-full" min={0.01} precision={2} />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={createMutation.isPending} block>
          创建并打开
        </Button>
      </Form>
    </Modal>
  )
}
