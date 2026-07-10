/// <reference types="node" />

import { execFileSync } from 'node:child_process'
import { mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import type { RJSFSchema } from '@rjsf/utils'
import { describe, expect, it } from 'vitest'

import { jsonSchemaValidator } from '#lib/json-schema-validator'

type Descriptor = { Key: string; JSONSchema?: RJSFSchema; ConfigSchema?: RJSFSchema }

describe('Provider schema cross-runtime contract', () => {
  it('Ajv2020 一次编译全部 Go/Invopop 实际产物', () => {
    const directory = mkdtempSync(join(tmpdir(), 'moonbase-provider-schema-'))
    const output = join(directory, 'all.json')
    try {
      execFileSync(
        'go',
        ['test', '.', '-run', '^TestExportAllProviderSchemasForWeb$', '-count=1'],
        {
          cwd: resolve(process.cwd(), '../../packages/integrations'),
          env: { ...process.env, MOONBASE_PROVIDER_SCHEMA_OUTPUT: output },
          stdio: 'pipe',
        },
      )
      const groups = JSON.parse(readFileSync(output, 'utf8')) as Record<string, Descriptor[]>
      expect(
        Object.fromEntries(
          Object.entries(groups).map(([key, values]) => [key, values.map((v) => v.Key)]),
        ),
      ).toEqual({
        email: ['smtp', 'cloudflare'],
        payment: ['alipay', 'wechat'],
        oauth: ['oidc', 'wechat'],
        sms: ['aliyun', 'tencent'],
        storage: ['s3', 'local'],
        captcha: ['turnstile', 'geetest', 'altcha'],
        llm: ['openai', 'anthropic'],
      })
      for (const descriptors of Object.values(groups)) {
        for (const descriptor of descriptors) {
          const schema = descriptor.JSONSchema ?? descriptor.ConfigSchema
          if (!schema) throw new Error(`provider ${descriptor.Key} 缺少 schema`)
          expect(() => jsonSchemaValidator.rawValidation(schema, {})).not.toThrow()
        }
      }
    } finally {
      rmSync(directory, { recursive: true, force: true })
    }
  })
})
