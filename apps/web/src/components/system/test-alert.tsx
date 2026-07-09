import { Alert } from 'antd'

export type TestState = { ok: boolean; message: string } | undefined

// The wire message carries failure diagnostics (untranslated, per the
// backend-errors-stay-English rule) or the LLM's actual reply; success
// boilerplate is empty. The verdict line is ours to translate.
export function TestAlert({ result }: { result: TestState }) {
  if (!result) return null
  return (
    <Alert
      className="mb-4"
      type={result.ok ? 'success' : 'error'}
      title={result.ok ? '测试通过' : '测试失败'}
      description={result.message || undefined}
      showIcon
    />
  )
}
