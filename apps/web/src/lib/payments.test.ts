import { describe, expect, it } from 'vitest'
import { METHOD_INPUTS, methodInputs, PROVIDER_METHODS } from '#lib/payments'

// Behavioral spec for the generated payment method catalog (protoc-gen-paymentcatalog,
// re-exported through payments.ts). The Go and TS catalogs are generated from the
// one payment.v1.PaymentMethod proto enum, so they can't drift — this pins the
// generated TS values to the established set so a generation regression fails here
// instead of silently mis-rendering the checkout.
describe('generated payment method catalog', () => {
  it('groups product ids by provider in display order', () => {
    expect(PROVIDER_METHODS).toEqual({
      alipay: ['precreate', 'page_pay', 'wap_pay', 'create', 'app_pay'],
      wechat: ['native', 'h5', 'jsapi', 'app'],
    })
  })

  it('collects payer_id / return_url only where the product needs it', () => {
    expect(methodInputs('page_pay')).toEqual(['return_url'])
    expect(methodInputs('wap_pay')).toEqual(['return_url'])
    expect(methodInputs('create')).toEqual(['payer_id'])
    expect(methodInputs('jsapi')).toEqual(['payer_id'])
    expect(methodInputs('precreate')).toEqual([])
    expect(methodInputs('native')).toEqual([])
  })

  it('has an inputs entry for every catalogued product', () => {
    for (const id of Object.values(PROVIDER_METHODS).flat()) {
      expect(METHOD_INPUTS[id]).toBeDefined()
    }
  })
})
