import { create } from '@bufbuild/protobuf'
import {
  CheckoutOrderSchema,
  CheckoutPaymentMethodSchema,
  CheckoutSessionSchema,
  PaymentActionSchema,
  PaymentOrderSchema,
  PresentationSchema,
} from '@moonbase/api-client'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { CheckoutSummary, PaymentActionView } from '#components/payment-checkout'

describe('Hosted checkout', () => {
  it('只向付款人展示支付方式，不暴露内部路由', () => {
    const session = create(CheckoutSessionSchema, {
      subject: '会员续费',
      amount: 12800n,
      status: 'open',
      paymentMethods: [
        create(CheckoutPaymentMethodSchema, {
          key: 'wechat-pay',
          presentation: create(PresentationSchema, {
            name: '微信支付',
            description: '使用微信完成付款',
          }),
        }),
      ],
    })

    const html = renderToStaticMarkup(
      <CheckoutSummary session={session} selectedMethod="" onSelect={() => undefined} />,
    )

    expect(html).toContain('会员续费')
    expect(html).toContain('¥128.00')
    expect(html).toContain('微信支付')
    expect(html).toContain('使用微信完成付款')
    expect(html).not.toContain('profile')
    expect(html).not.toContain('native')
  })

  it('按通用动作渲染二维码和已完成结果', () => {
    const qr = create(PaymentActionSchema, {
      action: { case: 'qr', value: { data: 'https://pay.example/qr' } },
    })
    const paid = create(CheckoutOrderSchema, {
      order: create(PaymentOrderSchema, { status: 'paid' }),
    })

    const actionHTML = renderToStaticMarkup(<PaymentActionView action={qr} />)
    const paidHTML = renderToStaticMarkup(
      <PaymentActionView checkoutOrder={paid} returnPath="/payments" />,
    )

    expect(actionHTML).toContain('请扫码支付')
    expect(paidHTML).toContain('支付成功')
    expect(paidHTML).toContain('返回业务页面')
    expect(paidHTML).toContain('/payments')
  })
})
