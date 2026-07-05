import { m } from '#paraglide/messages.js'

// Official payment product ids per provider, in display order. Mirrors the Go
// driver catalog in apps/server/internal/pay (alipay.go / wechat.go).
export const PROVIDER_METHODS: Record<string, string[]> = {
  alipay: ['precreate', 'page_pay', 'wap_pay', 'create', 'app_pay'],
  wechat: ['native', 'h5', 'jsapi', 'app'],
}

export const METHOD_LABEL: Record<string, () => string> = {
  precreate: m.payments_method_precreate,
  page_pay: m.payments_method_page_pay,
  wap_pay: m.payments_method_wap_pay,
  create: m.payments_method_create,
  app_pay: m.payments_method_app_pay,
  native: m.payments_method_native,
  h5: m.payments_method_h5,
  jsapi: m.payments_method_jsapi,
  app: m.payments_method_app,
}

export const METHOD_DESC: Record<string, () => string> = {
  precreate: m.payments_method_precreate_desc,
  page_pay: m.payments_method_page_pay_desc,
  wap_pay: m.payments_method_wap_pay_desc,
  create: m.payments_method_create_desc,
  app_pay: m.payments_method_app_pay_desc,
  native: m.payments_method_native_desc,
  h5: m.payments_method_h5_desc,
  jsapi: m.payments_method_jsapi_desc,
  app: m.payments_method_app_desc,
}

// Extra per-order inputs a product collects, mirroring pay.Method.Inputs.
export const METHOD_INPUTS: Record<string, string[]> = {
  precreate: [],
  page_pay: ['return_url'],
  wap_pay: ['return_url'],
  create: ['payer_id'],
  app_pay: [],
  native: [],
  h5: [],
  jsapi: ['payer_id'],
  app: [],
}

export const methodLabel = (id: string) => METHOD_LABEL[id]?.() ?? id
export const methodDesc = (id: string) => METHOD_DESC[id]?.() ?? ''
export const methodInputs = (id: string) => METHOD_INPUTS[id] ?? []
