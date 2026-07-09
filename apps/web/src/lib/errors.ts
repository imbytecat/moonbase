import { Code, ConnectError } from '@connectrpc/connect'

// Maps backend errors (English diagnostics) to user-facing text at the
// display boundary. Matching on rawMessage strings is intentionally shallow:
// unknown errors fall back to a generic message rather than leaking English.
const messageMap: Array<{ match: RegExp; message: () => string }> = [
  { match: /invalid (email or password|credentials)/i, message: () => '邮箱或密码错误' },
  { match: /captcha verification failed/i, message: () => '人机验证未通过，请重试' },
  { match: /invalid or expired code/i, message: () => '验证码错误或已过期' },
  { match: /too many requests/i, message: () => '请求过于频繁，请稍后再试' },
  { match: /invalid phone number/i, message: () => '手机号格式不正确' },
  { match: /region not supported/i, message: () => '暂不支持该地区的手机号' },
  { match: /already bound/i, message: () => '该手机号已绑定其他账号' },
  { match: /already registered/i, message: () => '该邮箱已被注册' },
  { match: /registration is disabled/i, message: () => '该功能暂不可用' },
  { match: /not available|not configured/i, message: () => '该功能暂不可用' },
]

export function humanizeError(err: unknown): string {
  if (err instanceof ConnectError) {
    for (const { match, message } of messageMap) {
      if (match.test(err.rawMessage)) return message()
    }
    switch (err.code) {
      case Code.Unauthenticated:
        return '登录已过期，请重新登录'
      case Code.PermissionDenied:
        return '您没有权限访问此页面'
      case Code.ResourceExhausted:
        return '请求过于频繁，请稍后再试'
      default:
        return '请求失败，请稍后重试'
    }
  }
  return '请求失败，请稍后重试'
}
