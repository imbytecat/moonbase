import { create } from '@bufbuild/protobuf'
import { ConfigViewSchema, ProfileSchema, ProviderFormSchema } from '@moonbase/api-client'
import type { RJSFSchema } from '@rjsf/utils'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'

import {
  ConfigForm,
  prepareConfigSchema,
  prepareConfigUiSchema,
  splitConfigWrite,
} from '#components/system/config-form'
import { jsonSchemaValidator } from '#lib/json-schema-validator'

describe('ConfigForm', () => {
  const schema = {
    $schema: 'https://json-schema.org/draft/2020-12/schema',
    type: 'object',
    additionalProperties: false,
    properties: {
      host: { type: 'string', title: '服务器地址', minLength: 1 },
      password: { type: 'string', title: '密码', minLength: 1, writeOnly: true },
    },
    required: ['host', 'password'],
  }

  it('使用 secret presence 渲染留空保留提示', () => {
    const html = renderToStaticMarkup(
      <ConfigForm
        provider="smtp"
        providerForm={create(ProviderFormSchema, {
          schema,
          uiSchema: {
            password: {
              'ui:widget': 'secret',
              'ui:options': { secret: true },
            },
          },
        })}
        profile={create(ProfileSchema, {
          id: 'profile-1',
          name: '主邮件',
          provider: 'smtp',
          config: create(ConfigViewSchema, {
            values: { host: 'smtp.example.com' },
            setSecretPaths: ['/password'],
          }),
          configValid: true,
        })}
        saving={false}
        onSubmit={() => undefined}
      />,
    )

    expect(html).toContain('smtp.example.com')
    expect(html).toContain('留空保持不变')
  })

  it('使用 Ajv2020 编译 provider schema', () => {
    const result = jsonSchemaValidator.rawValidation(schema as RJSFSchema, {
      host: 'smtp.example.com',
      password: 'secret',
    })
    expect(result.errors).toBeUndefined()
  })

  it('编辑时从 dependentRequired 移除已存 secret', () => {
    const editSchema = prepareConfigSchema(
      {
        type: 'object',
        properties: {
          username: { type: 'string' },
          password: { type: 'string' },
        },
        required: ['username', 'password'],
        dependentRequired: { username: ['password'], password: ['username'] },
      },
      true,
      new Set(['/password']),
      '配置名称',
    )

    const result = jsonSchemaValidator.rawValidation(editSchema, {
      name: '主邮件',
      username: 'mailer',
    })
    expect(result.errors).toBeUndefined()
  })

  it('编辑时仍要求补齐尚未设置的 required secret', () => {
    const editSchema = prepareConfigSchema(
      {
        type: 'object',
        properties: { password: { type: 'string', minLength: 1 } },
        required: ['password'],
      },
      true,
      new Set(),
      '配置名称',
    )
    const result = jsonSchemaValidator.rawValidation(editSchema, { name: '待修复邮件' })
    expect(result.errors).toHaveLength(1)
  })

  it('只禁用已有值的 create-only 字段', () => {
    const ui = { key: { 'ui:options': { immutable: true } } }
    const missing = prepareConfigUiSchema(
      ui,
      create(ProfileSchema, { config: create(ConfigViewSchema, { values: {} }) }),
      {},
    )
    const existing = prepareConfigUiSchema(
      ui,
      create(ProfileSchema, {
        config: create(ConfigViewSchema, { values: { key: 'stable' } }),
      }),
      {},
    )
    expect(missing.uiSchema.key?.['ui:disabled']).toBeUndefined()
    expect(existing.uiSchema.key?.['ui:disabled']).toBe(true)
  })

  it('按嵌套 JSON Pointer 拆分普通值与 secret', () => {
    expect(
      splitConfigWrite(
        {
          credentials: { username: 'moonbase', token: 'secret' },
        },
        new Set(['/credentials/token']),
      ),
    ).toEqual({
      values: { credentials: { username: 'moonbase' } },
      secrets: { '/credentials/token': 'secret' },
    })
  })
})
