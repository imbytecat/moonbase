import { Turnstile } from '@marsidev/react-turnstile'
import { GeeTest } from 'react-geetest-v4'
import { AltchaWidget } from './altcha-widget'

interface CaptchaWidgetProps {
  provider: string
  siteKey: string
  onToken: (token: string) => void
}

// Renders the widget for whichever provider the server reports via
// GetAuthConfig and pushes the response token up (Geetest tokens are the
// validate JSON, matching what the backend verifier expects). Renders nothing
// when no provider is configured.
export function CaptchaWidget({ provider, siteKey, onToken }: CaptchaWidgetProps) {
  if (provider === 'turnstile' && siteKey) {
    return (
      <div className="mb-4 flex justify-center">
        <Turnstile
          siteKey={siteKey}
          onSuccess={onToken}
          onExpire={() => onToken('')}
          onError={() => onToken('')}
        />
      </div>
    )
  }
  if (provider === 'geetest' && siteKey) {
    return (
      <div className="mb-4 flex justify-center">
        <GeeTest
          captchaId={siteKey}
          product="float"
          onSuccess={(result) => onToken(result ? JSON.stringify(result) : '')}
          onError={() => onToken('')}
          onClose={() => onToken('')}
        />
      </div>
    )
  }
  if (provider === 'altcha') {
    return (
      <div className="mb-4 flex justify-center">
        <AltchaWidget onToken={onToken} />
      </div>
    )
  }
  return null
}
