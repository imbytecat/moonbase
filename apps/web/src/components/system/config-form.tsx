import type { JsonObject } from '@bufbuild/protobuf'
import type { Profile, ProviderForm } from '@moonbase/api-client'
import Form from '@rjsf/antd'
import type { RegistryWidgetsType, RJSFSchema, UiSchema, WidgetProps } from '@rjsf/utils'
import { Button, Input, Select } from 'antd'
import { type ReactNode, useMemo, useState } from 'react'

import { jsonSchemaValidator } from '#lib/json-schema-validator'

export interface ConfigProfileInput {
  id: string
  name: string
  provider: string
  config: {
    values: JsonObject
    secrets: Record<string, string>
  }
}

export interface NameField {
  label?: string
  placeholder?: string
  help?: string
}

// Secret values are write-only. setSecretPaths only reports whether a stored
// value exists; an empty edit keeps it and a non-empty edit replaces it.
function SecretWidget({ id, value, onChange, disabled, placeholder, options }: WidgetProps) {
  const kept = options?.secretSet === true
  return (
    <Input.Password
      id={id}
      autoComplete="new-password"
      disabled={disabled}
      value={typeof value === 'string' ? value : ''}
      placeholder={kept ? '留空保持不变' : (placeholder as string | undefined)}
      onChange={(event) => onChange(event.target.value)}
    />
  )
}

// Renders enum / string_array options as label over a muted description; the
// selected value shows the plain title (optionLabelProp), never the two-line
// block. Driver-written descriptions come through ui:options.descriptions.
function OptionSelectWidget({
  value,
  onChange,
  disabled,
  placeholder,
  multiple,
  schema,
  options,
}: WidgetProps) {
  const isMultiple = multiple === true || schema.type === 'array'
  const enumOptions = (options?.enumOptions as { value: string; label: string }[] | undefined) ?? []
  const descriptions = (options?.descriptions as Record<string, string> | undefined) ?? {}
  const items = enumOptions.map((option) => {
    const desc = descriptions[option.value]
    return {
      value: option.value,
      title: option.label,
      label: desc ? (
        <div>
          <div>{option.label}</div>
          <div className="text-xs text-(--ant-color-text-tertiary)">{desc}</div>
        </div>
      ) : (
        option.label
      ),
    }
  })
  return (
    <Select
      className="w-full"
      mode={isMultiple ? 'multiple' : undefined}
      disabled={disabled}
      allowClear
      placeholder={placeholder as string | undefined}
      optionLabelProp="title"
      showSearch={{ optionFilterProp: 'title' }}
      maxTagCount={isMultiple ? 'responsive' : undefined}
      value={value}
      onChange={(next) => onChange(next)}
      options={items}
    />
  )
}

const widgets: RegistryWidgetsType = { secret: SecretWidget, optionSelect: OptionSelectWidget }

function pointerForKey(key: string): string {
  return `/${key.replaceAll('~', '~0').replaceAll('/', '~1')}`
}

function initialFormData(profile: Profile | undefined): JsonObject {
  const out: JsonObject = { name: profile?.name ?? '' }
  for (const [key, val] of Object.entries(profile?.config?.values ?? {})) {
    out[key] = val
  }
  return out
}

function prepareUiSchema(
  base: UiSchema,
  profile: Profile | undefined,
  nameField: NameField,
): { uiSchema: UiSchema; secretKeys: Set<string> } {
  const isEdit = profile !== undefined
  const secretKeys = new Set<string>()
  const order = (base['ui:order'] as string[] | undefined) ?? []
  const nameUi: Record<string, unknown> = {
    'ui:placeholder': nameField.placeholder ?? '便于识别的名称',
  }
  if (nameField.help) nameUi['ui:help'] = nameField.help
  const uiSchema: UiSchema = { name: nameUi }
  if (order.length > 0) uiSchema['ui:order'] = ['name', ...order]
  for (const [key, raw] of Object.entries(base)) {
    if (key === 'ui:order') continue
    const entry = { ...(raw as Record<string, unknown>) }
    const opts = { ...((entry['ui:options'] as Record<string, unknown> | undefined) ?? {}) }
    if (opts.secret) {
      secretKeys.add(key)
      opts.secretSet = profile?.config?.setSecretPaths.includes(pointerForKey(key)) === true
    }
    if (opts.immutable && isEdit) entry['ui:disabled'] = true
    entry['ui:options'] = opts
    uiSchema[key] = entry
  }
  return { uiSchema, secretKeys }
}

function prepareSchema(
  base: RJSFSchema,
  isEdit: boolean,
  secretKeys: Set<string>,
  nameLabel: string,
): RJSFSchema {
  const source = base as {
    properties?: Record<string, unknown>
    required?: string[]
    allOf?: unknown[]
  }
  const properties = { name: { type: 'string', title: nameLabel }, ...(source.properties ?? {}) }
  const required = ['name', ...(source.required ?? [])]
  const schema = { ...(base as object), type: 'object', properties, required } as RJSFSchema
  if (!isEdit) return schema
  // Editing a masked secret may leave it blank to keep the stored value, so a
  // secret is not required on edit — drop it from every required list.
  schema.required = required.filter((key) => !secretKeys.has(key))
  if (Array.isArray(source.allOf)) {
    schema.allOf = source.allOf.map((clause) => {
      const cloned = structuredClone(clause) as { then?: { required?: string[] } }
      if (cloned.then?.required) {
        cloned.then.required = cloned.then.required.filter((key) => !secretKeys.has(key))
      }
      return cloned
    }) as RJSFSchema['allOf']
  }
  return schema
}

export function ConfigForm({
  providerForm,
  provider,
  profile,
  saving,
  onSubmit,
  onDirtyChange,
  nameField = {},
  banner,
  actions,
}: {
  providerForm: ProviderForm
  provider: string
  profile: Profile | undefined
  saving: boolean
  onSubmit: (input: ConfigProfileInput) => void
  onDirtyChange?: (dirty: boolean) => void
  nameField?: NameField
  banner?: (current: ConfigProfileInput) => ReactNode
  actions?: (current: ConfigProfileInput) => ReactNode
}) {
  const isEdit = profile !== undefined
  const { uiSchema, secretKeys } = useMemo(
    () => prepareUiSchema((providerForm.uiSchema ?? {}) as UiSchema, profile, nameField),
    [providerForm, profile, nameField],
  )
  const schema = useMemo(
    () =>
      prepareSchema(
        (providerForm.schema ?? {}) as RJSFSchema,
        isEdit,
        secretKeys,
        nameField.label ?? '配置名称',
      ),
    [providerForm, isEdit, secretKeys, nameField.label],
  )
  const initialJson = useMemo(() => JSON.stringify(initialFormData(profile)), [profile])
  const [formData, setFormData] = useState<JsonObject>(() => initialFormData(profile))

  const toInput = (data: JsonObject): ConfigProfileInput => {
    const { name, ...rawConfig } = data
    const values: JsonObject = {}
    const secrets: Record<string, string> = {}
    for (const [key, value] of Object.entries(rawConfig)) {
      if (!secretKeys.has(key)) {
        values[key] = value
        continue
      }
      if (typeof value === 'string' && value !== '') secrets[pointerForKey(key)] = value
    }
    return {
      id: profile?.id ?? '',
      name: typeof name === 'string' ? name : '',
      provider,
      config: { values, secrets },
    }
  }

  return (
    <Form
      schema={schema}
      uiSchema={uiSchema}
      formData={formData}
      validator={jsonSchemaValidator}
      widgets={widgets}
      showErrorList={false}
      onChange={(event) => {
        const next = (event.formData ?? {}) as JsonObject
        setFormData(next)
        onDirtyChange?.(JSON.stringify(next) !== initialJson)
      }}
      onSubmit={(event) => onSubmit(toInput((event.formData ?? {}) as JsonObject))}
    >
      <div className="mt-3 space-y-3">
        {banner?.(toInput(formData))}
        <div className="flex flex-wrap items-center gap-2">
          <Button type="primary" htmlType="submit" loading={saving}>
            {'保存'}
          </Button>
          {actions?.(toInput(formData))}
        </div>
      </div>
    </Form>
  )
}

export function SchemaForm({
  form,
  saving,
  submitText = '继续支付',
  onSubmit,
}: {
  form: ProviderForm
  saving: boolean
  submitText?: string
  onSubmit: (data: JsonObject) => void
}) {
  const [formData, setFormData] = useState<JsonObject>({})
  return (
    <Form
      schema={(form.schema ?? {}) as RJSFSchema}
      uiSchema={(form.uiSchema ?? {}) as UiSchema}
      formData={formData}
      validator={jsonSchemaValidator}
      widgets={widgets}
      showErrorList={false}
      onChange={(event) => setFormData((event.formData ?? {}) as JsonObject)}
      onSubmit={(event) => onSubmit((event.formData ?? {}) as JsonObject)}
    >
      <Button type="primary" htmlType="submit" loading={saving} block>
        {submitText}
      </Button>
    </Form>
  )
}
