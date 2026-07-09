import type { JsonObject, JsonValue } from '@bufbuild/protobuf'
import type { FieldDescriptor, Profile } from '@moonbase/api-client'
import { Form, Input, InputNumber, Select, Switch } from 'antd'
import { m } from '#paraglide/messages.js'

export interface SchemaProfileFormValues {
  readonly name?: string
  readonly config?: Record<string, JsonValue>
}

export function schemaInitialConfig(
  profile: Profile | undefined,
  provider: string,
  fields: readonly FieldDescriptor[],
): JsonObject {
  const stored = profile?.provider === provider ? profile.config : undefined
  const out: JsonObject = {}
  for (const field of fields) {
    const value = stored?.[field.key]
    if (field.secret) {
      out[field.key] = ''
    } else if (field.type === 'bool') {
      out[field.key] = Boolean(value)
    } else if (field.type === 'int') {
      out[field.key] = typeof value === 'number' ? value : 0
    } else if (field.key === 'methods') {
      out[field.key] = stringsOf(value)
    } else {
      out[field.key] = typeof value === 'string' ? value : ''
    }
  }
  return out
}

export function schemaProfileToProto(
  profile: Profile | undefined,
  provider: string,
  fields: readonly FieldDescriptor[],
  values: SchemaProfileFormValues,
): Pick<Profile, 'id' | 'name' | 'provider' | 'config'> {
  const config: JsonObject = {}
  for (const field of fields) {
    const value = values.config?.[field.key]
    if (field.type === 'bool') {
      config[field.key] = Boolean(value)
    } else if (field.type === 'int') {
      config[field.key] = typeof value === 'number' ? value : 0
    } else if (field.key === 'methods') {
      config[field.key] = stringsOf(value)
    } else {
      config[field.key] = typeof value === 'string' ? value : ''
    }
  }
  return { id: profile?.id ?? '', name: values.name ?? '', provider, config }
}

export function SchemaField({
  field,
  profile,
}: {
  readonly field: FieldDescriptor
  readonly profile: Profile | undefined
}) {
  return (
    <Form.Item
      name={['config', field.key]}
      label={field.label}
      rules={field.required && !field.secret ? [{ required: true }] : []}
      valuePropName={field.type === 'bool' ? 'checked' : 'value'}
    >
      {fieldControl(
        field,
        profile?.config?.[`${field.key}_set`] === true,
        field.immutable && profile !== undefined,
      )}
    </Form.Item>
  )
}

function fieldControl(field: FieldDescriptor, secretSet: boolean, disabled: boolean) {
  if (field.secret) {
    return (
      <Input.Password
        autoComplete="new-password"
        disabled={disabled}
        placeholder={secretSet ? m.systemPage_secretUnchanged() : ''}
      />
    )
  }
  if (field.key === 'methods') {
    return (
      <Select
        mode="multiple"
        disabled={disabled}
        maxTagCount="responsive"
        options={field.options.map((option) => ({ value: option, label: option }))}
      />
    )
  }
  if (field.type === 'enum') {
    return (
      <Select
        disabled={disabled}
        options={field.options.map((option) => ({ value: option, label: option }))}
      />
    )
  }
  if (field.type === 'bool') return <Switch disabled={disabled} />
  if (field.type === 'int') return <InputNumber disabled={disabled} className="!w-full" />
  if (field.type === 'text') return <Input.TextArea disabled={disabled} autoSize={{ minRows: 2 }} />
  return <Input disabled={disabled} autoComplete="off" placeholder={field.help} />
}

function stringsOf(value: JsonValue | undefined): string[] {
  if (!Array.isArray(value)) return []
  return value.filter((item): item is string => typeof item === 'string')
}
