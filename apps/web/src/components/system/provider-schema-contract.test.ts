/// <reference types="node" />

import { execFileSync } from 'node:child_process'
import { mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import type { RJSFSchema } from '@rjsf/utils'
import { describe, expect, it } from 'vitest'

import { jsonSchemaValidator } from '#lib/json-schema-validator'

describe('Email provider schema contract', () => {
  it('Ajv2020 可编译 Go/Invopop 的实际产物', () => {
    const directory = mkdtempSync(join(tmpdir(), 'moonbase-provider-schema-'))
    const output = join(directory, 'email.json')
    try {
      execFileSync(
        'go',
        ['test', './email', '-run', '^TestExportProviderSchemasForWeb$', '-count=1'],
        {
          cwd: resolve(process.cwd(), '../../packages/integrations'),
          env: { ...process.env, MOONBASE_PROVIDER_SCHEMA_OUTPUT: output },
          stdio: 'pipe',
        },
      )
      const descriptors = JSON.parse(readFileSync(output, 'utf8')) as {
        Key: string
        JSONSchema: RJSFSchema
      }[]
      expect(descriptors.map((descriptor) => descriptor.Key)).toEqual(['smtp', 'cloudflare'])
      for (const descriptor of descriptors) {
        expect(() => jsonSchemaValidator.rawValidation(descriptor.JSONSchema, {})).not.toThrow()
      }
    } finally {
      rmSync(directory, { recursive: true, force: true })
    }
  })
})
