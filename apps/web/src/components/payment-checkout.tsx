import type { CheckoutOrder, CheckoutSession, PaymentAction } from '@moonbase/api-client'
import { Button, Card, QRCode, Result, Spin } from 'antd'
import { ProviderIcon } from '#components/provider-icon'

const terminalStatuses = new Set(['paid', 'failed', 'closed', 'refunded'])

export function isTerminalPaymentStatus(status: string | undefined) {
  return status !== undefined && terminalStatuses.has(status)
}

function yuan(cents: bigint) {
  return `¥${(Number(cents) / 100).toLocaleString('zh-CN', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`
}

export function CheckoutSummary({
  session,
  selectedMethod,
  onSelect,
}: {
  session: CheckoutSession
  selectedMethod: string
  onSelect: (method: string) => void
}) {
  return (
    <Card title="确认订单">
      <div className="mb-6 flex items-end justify-between gap-4">
        <div>
          <div className="text-sm text-(--ant-color-text-secondary)">订单内容</div>
          <div className="mt-1 text-lg font-medium">{session.subject}</div>
        </div>
        <div className="text-2xl font-semibold text-(--ant-color-primary)">
          {yuan(session.amount)}
        </div>
      </div>
      <div className="mb-3 font-medium">选择支付方式</div>
      <div className="space-y-3">
        {session.paymentMethods.map((method) => (
          <Button
            key={method.key}
            type={selectedMethod === method.key ? 'primary' : 'default'}
            className="h-auto min-h-16 justify-start py-3 text-left"
            block
            onClick={() => onSelect(method.key)}
          >
            <span className="flex items-center gap-3">
              <ProviderIcon iconRef={method.presentation?.iconRef ?? ''} />
              <span>
                <span className="block font-medium">{method.presentation?.name || method.key}</span>
                {method.presentation?.description ? (
                  <span className="block text-xs opacity-75">
                    {method.presentation.description}
                  </span>
                ) : null}
              </span>
            </span>
          </Button>
        ))}
      </div>
    </Card>
  )
}

export function PaymentActionView({
  action,
  checkoutOrder,
  returnPath,
}: {
  action?: PaymentAction
  checkoutOrder?: CheckoutOrder
  returnPath?: string
}) {
  const order = checkoutOrder?.order
  const returnButton = returnPath ? (
    <Button type="primary" href={returnPath}>
      返回业务页面
    </Button>
  ) : undefined

  switch (order?.status) {
    case 'paid':
      return <Result status="success" title="支付成功" extra={returnButton} />
    case 'refunded':
      return <Result status="success" title="退款已完成" extra={returnButton} />
    case 'failed':
      return (
        <Result
          status="error"
          title="支付创建失败"
          subTitle="请返回后重新发起支付"
          extra={returnButton}
        />
      )
    case 'closed':
      return <Result status="warning" title="支付已关闭" extra={returnButton} />
  }

  const next = action ?? checkoutOrder?.action
  switch (next?.action.case) {
    case 'qr':
      return (
        <div className="flex flex-col items-center gap-4 py-6">
          <QRCode value={next.action.value.data} />
          <div className="font-medium">请扫码支付</div>
          <div className="text-sm text-(--ant-color-text-secondary)">支付完成后页面会自动更新</div>
        </div>
      )
    case 'redirect':
      return (
        <div className="py-6 text-center">
          <Button type="primary" size="large" href={next.action.value.url}>
            前往支付
          </Button>
        </div>
      )
    case 'form': {
      const method = next.action.value.method.toLowerCase() === 'get' ? 'get' : 'post'
      return (
        <form action={next.action.value.url} method={method} className="py-6 text-center">
          {Object.entries(next.action.value.fields).map(([name, value]) => (
            <input key={name} type="hidden" name={name} value={value} />
          ))}
          <Button type="primary" size="large" htmlType="submit">
            前往支付
          </Button>
        </form>
      )
    }
    case 'hostedFlow':
      return (
        <div className="py-6 text-center">
          <Button type="primary" size="large" href={next.action.value.url} target="_blank">
            继续支付
          </Button>
        </div>
      )
    case 'wait':
    case undefined:
      return (
        <div className="flex flex-col items-center gap-4 py-8">
          <Spin size="large" />
          <div className="text-(--ant-color-text-secondary)">正在确认支付状态…</div>
        </div>
      )
  }
}
