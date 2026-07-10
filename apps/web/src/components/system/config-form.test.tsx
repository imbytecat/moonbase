import { create } from '@bufbuild/protobuf'
import { ConfigViewSchema, ProfileSchema, ProviderFormSchema } from '@moonbase/api-client'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'

import { ConfigForm } from '#components/system/config-form'
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
    const result = jsonSchemaValidator.rawValidation(schema, {
      host: 'smtp.example.com',
      password: 'secret',
    })
    expect(result.errors).toBeUndefined()
  })
})
