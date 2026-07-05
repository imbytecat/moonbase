import { Code, ConnectError } from '@connectrpc/connect'
import { m } from '#paraglide/messages.js'

// Maps backend errors (English diagnostics) to user-facing text at the
// display boundary. Matching on rawMessage strings is intentionally shallow:
// unknown errors fall back to a generic message rather than leaking English.
const messageMap: Array<{ match: RegExp; message: () => string }> = [
  { match: /invalid email or password/i, message: m.error_invalidCredentials },
  { match: /captcha verification failed/i, message: m.error_captchaFailed },
  { match: /invalid or expired code/i, message: m.error_invalidCode },
  { match: /too many requests/i, message: m.error_rateLimited },
  { match: /invalid phone number/i, message: m.error_phoneInvalid },
  { match: /region not supported/i, message: m.error_phoneRegionNotSupported },
  { match: /already bound/i, message: m.error_phoneAlreadyBound },
  { match: /already registered/i, message: m.error_emailAlreadyRegistered },
  { match: /registration is disabled/i, message: m.error_featureUnavailable },
  { match: /not available|not configured/i, message: m.error_featureUnavailable },
]

export function humanizeError(err: unknown): string {
  if (err instanceof ConnectError) {
    for (const { match, message } of messageMap) {
      if (match.test(err.rawMessage)) return message()
    }
    switch (err.code) {
      case Code.Unauthenticated:
        return m.error_unauthenticated()
      case Code.PermissionDenied:
        return m.error_forbidden()
      case Code.ResourceExhausted:
        return m.error_rateLimited()
      default:
        return m.error_generic()
    }
  }
  return m.error_generic()
}
