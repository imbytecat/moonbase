import { PlusOutlined } from '@ant-design/icons'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import {
  createConnectQueryKey,
  createQueryOptions,
  useMutation,
  useQuery,
  useSuspenseQuery,
} from '@connectrpc/connect-query'
import {
  createPaymentOrder,
  getMe,
  listPaymentOptions,
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
  Empty,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  QRCode,
  Segmented,
  Select,
  Table,
  Tag,
  Typography,
} from 'antd'
import { useState } from 'react'
import { humanizeError } from '#lib/errors'
import { methodDesc, methodInputs, methodLabel } from '#lib/payments'
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

const STATUS_COLORS: Record<string, string> = {
  created: 'blue',
  paid: 'green',
  closed: 'default',
  refunding: 'gold',
  refunded: 'purple',
}

const STATUS_LABELS: Record<string, () => string> = {
  created: () => '待支付',
  paid: () => '已支付',
  closed: () => '已关闭',
  refunding: () => '退款中',
  refunded: () => '已退款',
}

const PROVIDER_LABELS: Record<string, () => string> = {
  alipay: () => '支付宝',
  wechat: () => '微信支付',
}

const yuan = (cents: bigint) =>
  `¥${(Number(cents) / 100).toLocaleString(undefined, { minimumFractionDigits: 2 })}`

function PaymentsPage() {
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(0)
  const [status, setStatus] = useState('')
  const { data } = useSuspenseQuery(listPaymentOrders, { page, pageSize: 20, status })
  const { data: session } = useSuspenseQuery(getMe)
  const [creating, setCreating] = useState(false)
  const [checkoutId, setCheckoutId] = useState<string>()

  const canWrite = hasPermission(session?.user, Permission.PAYMENT_WRITE)

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listPaymentOrders, cardinality: 'finite' }),
    })

  const refundMutation = useMutation(refundPaymentOrder, {
    onSuccess: () => {
      void invalidate()
      message.success('退款已提交')
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  return (
    <div className="mx-auto max-w-6xl">
      <Card
        title={'支付订单'}
        extra={
          <div className="flex gap-2">
            <Select
              className="w-32"
              value={status}
              onChange={(v) => {
                setPage(0)
                setStatus(v)
              }}
              options={[
                { value: '', label: '全部状态' },
                ...Object.keys(STATUS_COLORS).map((s) => ({
                  value: s,
                  label: STATUS_LABELS[s]?.() ?? s,
                })),
              ]}
            />
            {canWrite ? (
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreating(true)}>
                {'创建演示订单'}
              </Button>
            ) : null}
          </div>
        }
      >
        {data.orders.length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={'暂无支付订单'} />
        ) : (
          <Table<PaymentOrder>
            rowKey="id"
            dataSource={data.orders}
            scroll={{ x: 'max-content' }}
            size="middle"
            pagination={{
              current: page + 1,
              pageSize: 20,
              total: Number(data.total),
              onChange: (p) => setPage(p - 1),
              showSizeChanger: false,
            }}
            columns={[
              {
                title: '商品',
                dataIndex: 'subject',
                render: (subject: string, order) => (
                  <div>
                    <Typography.Text strong className="text-[13px]">
                      {subject}
                    </Typography.Text>
                    <div className="text-xs text-(--ant-color-text-tertiary)">
                      {order.outTradeNo}
                    </div>
                  </div>
                ),
              },
              {
                title: '金额',
                dataIndex: 'amount',
                width: 120,
                render: (amount: bigint) => yuan(amount),
              },
              {
                title: '渠道',
                dataIndex: 'provider',
                width: 130,
                render: (provider: string, order) => (
                  <Tag>{`${PROVIDER_LABELS[provider]?.() ?? provider} · ${order.profileName}`}</Tag>
                ),
              },
              {
                title: '支付方式类型',
                dataIndex: 'method',
                width: 110,
                render: (method: string) => methodLabel(method),
              },
              {
                title: '状态',
                dataIndex: 'status',
                width: 110,
                render: (s: string) => (
                  <Tag color={STATUS_COLORS[s] ?? 'default'}>{STATUS_LABELS[s]?.() ?? s}</Tag>
                ),
              },
              {
                title: '创建时间',
                dataIndex: 'createdAt',
                width: 180,
                render: (_: unknown, order) =>
                  order.createdAt ? timestampDate(order.createdAt).toLocaleString() : '—',
              },
              {
                title: '操作',
                key: 'actions',
                width: 160,
                render: (_: unknown, order) => (
                  <div className="flex gap-2">
                    {canWrite && order.status === 'created' ? (
                      <Button size="small" onClick={() => setCheckoutId(order.id)}>
                        {'收款码'}
                      </Button>
                    ) : null}
                    {canWrite && order.status === 'paid' ? (
                      <Popconfirm
                        title={'全额退款该订单？'}
                        onConfirm={() => refundMutation.mutate({ id: order.id })}
                      >
                        <Button size="small" danger loading={refundMutation.isPending}>
                          {'退款'}
                        </Button>
                      </Popconfirm>
                    ) : null}
                  </div>
                ),
              },
            ]}
          />
        )}
      </Card>

      <CreateOrderModal
        open={creating}
        onClose={() => setCreating(false)}
        onCreated={(id) => {
          setCreating(false)
          void invalidate()
          setCheckoutId(id)
        }}
      />
      <CheckoutModal
        id={checkoutId}
        onClose={() => {
          setCheckoutId(undefined)
          void invalidate()
        }}
      />
    </div>
  )
}

function CreateOrderModal({
  open,
  onClose,
  onCreated,
}: {
  open: boolean
  onClose: () => void
  onCreated: (id: string) => void
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<{
    profileId: string
    method: string
    subject: string
    amount: number
    payerId?: string
    returnUrl?: string
  }>()
  const profileId = Form.useWatch('profileId', form)
  const method = Form.useWatch('method', form) ?? ''
  const { data: options } = useQuery(listPaymentOptions, { purpose: 'checkout' }, { enabled: open })

  const createMutation = useMutation(createPaymentOrder, {
    onSuccess: (res) => {
      if (res.order) onCreated(res.order.id)
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const opts = options?.options ?? []
  const selected = opts.find((o) => o.profileId === profileId)
  const availableMethods = selected?.methods ?? []
  const inputs = methodInputs(method)

  return (
    <Modal
      title={'创建演示订单'}
      open={open}
      onCancel={onClose}
      onOk={() => form.submit()}
      confirmLoading={createMutation.isPending}
      okButtonProps={{ disabled: opts.length === 0 }}
      destroyOnHidden
    >
      {opts.length === 0 ? (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={'收银台未绑定任何支付渠道——请先在设置中添加支付配置并绑定收银台'}
        />
      ) : (
        <Form
          form={form}
          layout="vertical"
          requiredMark={false}
          initialValues={{ subject: '演示商品', amount: 0.01 }}
          onFinish={(values) =>
            createMutation.mutate({
              purpose: 'checkout',
              profileId: values.profileId,
              subject: values.subject,
              amount: BigInt(Math.round(values.amount * 100)),
              method: values.method,
              payerId: values.payerId ?? '',
              returnUrl: values.returnUrl ?? '',
            })
          }
        >
          <Form.Item
            name="profileId"
            label={'支付方式'}
            rules={[{ required: true, message: '请选择支付方式' }]}
          >
            <Select
              onChange={(v) => {
                const opt = opts.find((o) => o.profileId === v)
                form.setFieldValue('method', opt?.methods?.[0] ?? '')
              }}
              options={opts.map((o) => ({
                value: o.profileId,
                label: `${PROVIDER_LABELS[o.provider]?.() ?? o.provider} · ${o.name}`,
              }))}
            />
          </Form.Item>
          <Form.Item
            name="method"
            label={'支付方式类型'}
            extra={method ? methodDesc(method) : '请先选择上方的支付渠道'}
          >
            <Segmented
              disabled={availableMethods.length === 0}
              options={availableMethods.map((id) => ({ value: id, label: methodLabel(id) }))}
            />
          </Form.Item>
          {inputs.includes('payer_id') ? (
            <Form.Item
              name="payerId"
              label={'付款人标识'}
              extra={'微信 openid 或支付宝买家 ID，需由客户端授权流程获取'}
              rules={[{ required: true }]}
            >
              <Input autoComplete="off" />
            </Form.Item>
          ) : null}
          {inputs.includes('return_url') ? (
            <Form.Item
              name="returnUrl"
              label={'支付完成回跳地址'}
              extra={'选填：付款完成后付款人浏览器跳转回的商户页面地址'}
            >
              <Input autoComplete="off" placeholder="https://" />
            </Form.Item>
          ) : null}
          <Form.Item name="subject" label={'商品'} rules={[{ required: true }]}>
            <Input maxLength={128} />
          </Form.Item>
          <Form.Item name="amount" label={'金额（元）'} rules={[{ required: true }]}>
            <InputNumber className="w-full" min={0.01} precision={2} prefix="¥" />
          </Form.Item>
        </Form>
      )}
    </Modal>
  )
}

// The QR stays up while the buyer scans; poll SyncPaymentOrder so the modal
// flips to the paid state without the async notification (which localhost
// deployments never receive).
const CHECKOUT_POLL_MS = 3000

function CheckoutModal({ id, onClose }: { id: string | undefined; onClose: () => void }) {
  const { data } = useQuery(syncPaymentOrder, id ? { id } : undefined, {
    enabled: Boolean(id),
    refetchInterval: (query) =>
      query.state.data?.order?.status === 'created' ? CHECKOUT_POLL_MS : false,
  })
  const order = data?.order

  return (
    <Modal title={'扫码支付'} open={Boolean(id)} onCancel={onClose} footer={null}>
      {order ? (
        <div className="flex flex-col items-center gap-4 py-4">
          <Typography.Text strong>{order.subject}</Typography.Text>
          <Typography.Title level={3} className="!my-0">
            {yuan(order.amount)}
          </Typography.Title>
          {order.status === 'created' && order.credential ? (
            <PaymentCredential order={order} />
          ) : (
            <Tag color={STATUS_COLORS[order.status] ?? 'default'} className="text-sm">
              {STATUS_LABELS[order.status]?.() ?? order.status}
            </Tag>
          )}
        </div>
      ) : null}
    </Modal>
  )
}

// Credential rendering is shaped by credentialKind: qr = QR code, redirect =
// a link the payer opens, params = the raw invocation params (a real client
// app feeds them to the provider SDK; the admin demo can only display them).
function PaymentCredential({ order }: { order: PaymentOrder }) {
  if (order.credentialKind === 'qr') {
    return (
      <>
        <QRCode value={order.credential} size={220} />
        <Typography.Text type="secondary" className="text-xs">
          {order.provider === 'alipay' ? '请使用支付宝扫码支付' : '请使用微信扫码支付'}
        </Typography.Text>
      </>
    )
  }
  if (order.credentialKind === 'redirect') {
    return (
      <>
        <Button type="primary" href={order.credential} target="_blank">
          {'打开支付页面'}
        </Button>
        <Typography.Text type="secondary" className="text-xs">
          {'请在手机浏览器中打开该链接完成支付'}
        </Typography.Text>
      </>
    )
  }
  return (
    <>
      <pre className="max-w-full overflow-auto rounded-lg bg-(--ant-color-fill-quaternary) p-3 text-xs">
        {order.credential}
      </pre>
      <Typography.Text type="secondary" className="text-xs">
        {'应用内支付参数需由客户端应用传给支付 SDK 调起，此处仅作展示'}
      </Typography.Text>
    </>
  )
}
