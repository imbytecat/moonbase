import type { Rule } from 'antd/es/form'
import AntdPhoneInput from 'antd-phone-input'
import type { PhoneNumber } from 'antd-phone-input/types'
import { m } from '#paraglide/messages.js'

interface PhoneInputProps {
  value?: string
  onChange?: (e164: string) => void
  allowedRegions: string[]
}

// Region policy → widget behavior: a single allowed region locks the flag
// (no dropdown); a short list restricts the dropdown; empty allows any
// country. Value contract with forms is always an E.164 string — the same
// shape the backend validates and stores.
export function PhoneInput({ value, onChange, allowedRegions }: PhoneInputProps) {
  const single = allowedRegions.length === 1 ? allowedRegions[0]?.toLowerCase() : undefined
  return (
    <AntdPhoneInput
      value={value}
      country={single ?? 'cn'}
      disableDropdown={Boolean(single)}
      onlyCountries={
        allowedRegions.length > 1 ? allowedRegions.map((r) => r.toLowerCase()) : undefined
      }
      enableSearch
      onChange={(phone: PhoneNumber) => {
        const joined = `+${phone.countryCode ?? ''}${phone.areaCode ?? ''}${phone.phoneNumber ?? ''}`
        onChange?.(joined.replaceAll(/[^\d+]/g, ''))
      }}
    />
  )
}

// antd Form rule reusing the widget's own libphonenumber-backed validity.
export function phoneRule(): Rule {
  return {
    validator: (_, raw: unknown) => {
      const value = typeof raw === 'string' ? raw : ''
      if (value.length >= 6 && /^\+\d+$/.test(value)) return Promise.resolve()
      return Promise.reject(new Error(m.auth_phoneRule()))
    },
  }
}
