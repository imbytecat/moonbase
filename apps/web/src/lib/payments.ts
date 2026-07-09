import { METHOD_INPUTS, PROVIDER_METHODS } from '@moonbase/api-client'

// PROVIDER_METHODS (product ids per provider, in display order) and METHOD_INPUTS
// (extra per-order inputs a product collects) are generated from the
// payment.v1.PaymentMethod proto enum (protoc-gen-paymentcatalog) and re-exported
// here — one source with the Go driver catalog, so they cannot drift. Only the
// label/desc text below stays in the local Chinese copy catalog, not the proto.
export { METHOD_INPUTS, PROVIDER_METHODS }

export const METHOD_LABEL: Record<string, () => string> = {
  precreate: () => '当面付 / 订单码（扫码）',
  page_pay: () => '电脑网站支付',
  wap_pay: () => '手机网站支付',
  create: () => '小程序支付（JSAPI）',
  app_pay: () => 'APP 支付',
  native: () => 'Native 扫码支付',
  h5: () => 'H5 支付',
  jsapi: () => 'JSAPI（公众号 / 小程序）',
  app: () => 'APP 支付',
}

export const METHOD_DESC: Record<string, () => string> = {
  precreate: () => '商家展示二维码，付款人用支付宝扫一扫完成支付（alipay.trade.precreate）',
  page_pay: () => 'PC 网页跳转支付宝收银台完成支付（alipay.trade.page.pay）',
  wap_pay: () => '手机浏览器唤起支付宝完成支付（alipay.trade.wap.pay）',
  create: () => '支付宝小程序内唤起收银台，需付款人标识与小程序 APPID（alipay.trade.create）',
  app_pay: () => '服务端下单返回订单串，App 端交给支付宝 SDK 调起（alipay.trade.app.pay）',
  native: () => '商家展示二维码，付款人用微信扫一扫完成支付',
  h5: () => '非微信内的手机浏览器唤起微信支付',
  jsapi: () => '微信内公众号或小程序唤起支付，需付款人 openid',
  app: () => '微信 APP 支付：服务端下单返回调起参数，App 端交给微信 SDK 唤起',
}

export const methodLabel = (id: string) => METHOD_LABEL[id]?.() ?? id
export const methodDesc = (id: string) => METHOD_DESC[id]?.() ?? ''
export const methodInputs = (id: string) => METHOD_INPUTS[id] ?? []
