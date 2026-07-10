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

function pointerForTokens(tokens: string[]): string {
  return tokens.map(pointerForKey).join('')
}

function pointerTokens(pointer: string): string[] {
  return pointer
    .slice(1)
    .split('/')
    .map((token) => token.replaceAll('~1', '/').replaceAll('~0', '~'))
}

function hasValueAtPointer(values: JsonObject | undefined, pointer: string): boolean {
  let current: unknown = values
  for (const token of pointerTokens(pointer)) {
    if (typeof current !== 'object' || current === null || Array.isArray(current)) return false
    if (!Object.hasOwn(current, token)) return false
    current = (current as JsonObject)[token]
  }
  return true
}

function initialFormData(profile: Profile | undefined): JsonObject {
  const out: JsonObject = { name: profile?.name ?? '' }
  for (const [key, val] of Object.entries(profile?.config?.values ?? {})) {
    out[key] = val
  }
  return out
}

export function prepareConfigUiSchema(
  base: UiSchema,
  profile: Profile | undefined,
  nameField: NameField,
): { uiSchema: UiSchema; secretPaths: Set<string>; keptSecretPaths: Set<string> } {
  const isEdit = profile !== undefined
  const secretPaths = new Set<string>()
  const keptSecretPaths = new Set(profile?.config?.setSecretPaths ?? [])
  const order = (base['ui:order'] as string[] | undefined) ?? []
  const nameUi: Record<string, unknown> = {
    'ui:placeholder': nameField.placeholder ?? '便于识别的名称',
  }
  if (nameField.help) nameUi['ui:help'] = nameField.help
  const visit = (raw: UiSchema, path: string[]): UiSchema => {
    const out: UiSchema = {}
    for (const [key, value] of Object.entries(raw)) {
      if (key.startsWith('ui:') || typeof value !== 'object' || value === null) {
        out[key] = value
        continue
      }
      out[key] = visit(value as UiSchema, [...path, key])
    }
    const opts = { ...((out['ui:options'] as Record<string, unknown> | undefined) ?? {}) }
    const pointer = pointerForTokens(path)
    if (opts.secret) {
      secretPaths.add(pointer)
      opts.secretSet = profile?.config?.setSecretPaths.includes(pointer) === true
    }
    if (opts.immutable && isEdit && hasValueAtPointer(profile.config?.values, pointer)) {
      out['ui:disabled'] = true
    }
    if (Object.keys(opts).length > 0) out['ui:options'] = opts as never
    return out
  }
  const uiSchema = visit(base, [])
  uiSchema.name = nameUi
  if (order.length > 0) uiSchema['ui:order'] = ['name', ...order]
  return { uiSchema, secretPaths, keptSecretPaths }
}

export function prepareConfigSchema(
  base: RJSFSchema,
  isEdit: boolean,
  secretPaths: Set<string>,
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

  const relax = (raw: RJSFSchema, path: string[]): RJSFSchema => {
    const out = structuredClone(raw) as RJSFSchema & {
      dependentRequired?: Record<string, string[]>
      dependentSchemas?: Record<string, RJSFSchema | boolean>
      properties?: Record<string, RJSFSchema>
    }
    if (out.required) {
      out.required = out.required.filter(
        (key) => !secretPaths.has(pointerForTokens([...path, key])),
      )
    }
    if (out.dependentRequired) {
      for (const [key, dependencies] of Object.entries(out.dependentRequired)) {
        out.dependentRequired[key] = dependencies.filter(
          (dependency) => !secretPaths.has(pointerForTokens([...path, dependency])),
        )
      }
    }
    if (out.properties) {
      for (const [key, child] of Object.entries(out.properties)) {
        if (typeof child === 'object') out.properties[key] = relax(child, [...path, key])
      }
    }
    if (out.dependentSchemas) {
      for (const [key, child] of Object.entries(out.dependentSchemas)) {
        if (typeof child === 'object') out.dependentSchemas[key] = relax(child, path)
      }
    }
    for (const keyword of ['allOf', 'anyOf', 'oneOf'] as const) {
      const branches = out[keyword]
      if (Array.isArray(branches)) {
        out[keyword] = branches.map((branch) =>
          typeof branch === 'object' ? relax(branch, path) : branch,
        )
      }
    }
    for (const keyword of ['then', 'else'] as const) {
      const branch = out[keyword]
      if (branch && typeof branch === 'object') out[keyword] = relax(branch, path)
    }
    return out
  }
  return relax(schema, [])
}

export function splitConfigWrite(
  config: JsonObject,
  secretPaths: Set<string>,
): { values: JsonObject; secrets: Record<string, string> } {
  const values = structuredClone(config)
  const secrets: Record<string, string> = {}
  for (const path of secretPaths) {
    const tokens = pointerTokens(path)
    let current: JsonObject | undefined = values
    for (const token of tokens.slice(0, -1)) {
      const next: unknown = current?.[token]
      if (typeof next !== 'object' || next === null || Array.isArray(next)) {
        current = undefined
        break
      }
      current = next as JsonObject
    }
    if (!current) continue
    const key = tokens.at(-1)
    if (key === undefined) continue
    const value = current[key]
    delete current[key]
    if (typeof value === 'string' && value !== '') secrets[path] = value
  }
  return { values, secrets }
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
  const { uiSchema, secretPaths, keptSecretPaths } = useMemo(
    () => prepareConfigUiSchema((providerForm.uiSchema ?? {}) as UiSchema, profile, nameField),
    [providerForm, profile, nameField],
  )
  const schema = useMemo(
    () =>
      prepareConfigSchema(
        (providerForm.schema ?? {}) as RJSFSchema,
        isEdit,
        keptSecretPaths,
        nameField.label ?? '配置名称',
      ),
    [providerForm, isEdit, keptSecretPaths, nameField.label],
  )
  const initialJson = useMemo(() => JSON.stringify(initialFormData(profile)), [profile])
  const [formData, setFormData] = useState<JsonObject>(() => initialFormData(profile))

  const toInput = (data: JsonObject): ConfigProfileInput => {
    const { name, ...rawConfig } = data
    const config = splitConfigWrite(rawConfig, secretPaths)
    return {
      id: profile?.id ?? '',
      name: typeof name === 'string' ? name : '',
      provider,
      config,
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
