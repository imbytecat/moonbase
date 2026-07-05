import { METHOD_INPUTS, PROVIDER_METHODS } from '@moonbase/api-client'
import { m } from '#paraglide/messages.js'

// PROVIDER_METHODS (product ids per provider, in display order) and METHOD_INPUTS
// (extra per-order inputs a product collects) are generated from the
// payment.v1.PaymentMethod proto enum (protoc-gen-paymentcatalog) and re-exported
// here — one source with the Go driver catalog, so they cannot drift. Only the
// label/desc text below stays in the i18n catalog, not the proto.
export { METHOD_INPUTS, PROVIDER_METHODS }

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

export const methodLabel = (id: string) => METHOD_LABEL[id]?.() ?? id
export const methodDesc = (id: string) => METHOD_DESC[id]?.() ?? ''
export const methodInputs = (id: string) => METHOD_INPUTS[id] ?? []
