import { getOption, hasOption } from '@bufbuild/protobuf'
import { fieldRulesExtension, PaymentProfileSchema } from '@moonbase/api-client'
import { describe, expect, it } from 'vitest'
import { METHOD_DESC, METHOD_INPUTS, METHOD_LABEL, PROVIDER_METHODS } from '#lib/payments'

// Drift-gate: the hand-written payment-method catalog (payments.ts) mirrors the
// Go driver catalog. It can't read Go, but the proto `methods` field pins the
// same union as a buf.validate `repeated.items.string.in` rule (the Go guardrail
// TestPaymentProfileMethodsMatchContract keeps that == pay.Methods()). Reading
// that rule off the descriptor chains the frontend catalog to the one source.
function contractMethods(): string[] {
  const field = PaymentProfileSchema.fields.find((f) => f.name === 'methods')
  if (!field) throw new Error('PaymentProfile has no `methods` field')
  if (!hasOption(field, fieldRulesExtension)) {
    throw new Error('PaymentProfile.methods carries no buf.validate rule (options stripped?)')
  }
  const rules = getOption(field, fieldRulesExtension)
  const items = rules.type?.case === 'repeated' ? rules.type.value.items : undefined
  const values = items?.type?.case === 'string' ? items.type.value.in : undefined
  if (!values || values.length === 0) {
    throw new Error('PaymentProfile.methods has no string `in:` values')
  }
  return [...values].sort()
}

describe('payment method catalog mirror tracks the proto contract', () => {
  const contract = contractMethods()

  it('PROVIDER_METHODS union equals the proto methods in: constraint', () => {
    const mirror = [...new Set(Object.values(PROVIDER_METHODS).flat())].sort()
    expect(mirror).toEqual(contract)
  })

  it('every contract method has an inputs, label and description entry', () => {
    for (const method of contract) {
      expect(Object.keys(METHOD_INPUTS)).toContain(method)
      expect(METHOD_LABEL[method]).toBeDefined()
      expect(METHOD_DESC[method]).toBeDefined()
    }
  })
})
