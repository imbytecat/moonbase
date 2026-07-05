import { describe, expect, it } from 'vitest'
import en from '#messages/en.json'
import zhCN from '#messages/zh-CN.json'

// Paraglide already fails the compile on syntax errors, but it silently
// falls back to the base locale for missing keys — parity is enforced here.
function keys(catalog: Record<string, unknown>): string[] {
  return Object.keys(catalog)
    .filter((k) => !k.startsWith('$'))
    .sort()
}

function placeholders(value: string): string[] {
  return [...value.matchAll(/\{(\w+)\}/g)].map((match) => match[1] ?? '').sort()
}

describe('message catalogs', () => {
  it('en has exactly the same keys as zh-CN', () => {
    expect(keys(en)).toEqual(keys(zhCN))
  })

  it('interpolation placeholders match between locales', () => {
    for (const key of keys(en)) {
      const enValue = (en as Record<string, string>)[key] ?? ''
      const zhValue = (zhCN as Record<string, string>)[key] ?? ''
      expect(placeholders(enValue), `placeholder mismatch at ${key}`).toEqual(placeholders(zhValue))
    }
  })

  it('no empty translations', () => {
    for (const catalog of [en, zhCN]) {
      for (const key of keys(catalog)) {
        expect(
          String((catalog as Record<string, string>)[key]).trim(),
          `empty translation at ${key}`,
        ).not.toBe('')
      }
    }
  })
})
