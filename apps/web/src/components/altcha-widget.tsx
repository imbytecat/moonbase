import 'altcha'
import 'altcha/i18n/zh-cn'
import { useEffect, useRef } from 'react'

// The built-in ALTCHA proof-of-work widget (web component): fetches its
// challenge from the public endpoint, solves it in a worker, and reports the
// base64 payload as the captcha token. Language auto-detects from <html lang>
// with the zh-CN pack imported to cover both app locales.
export function AltchaWidget({ onToken }: { onToken: (token: string) => void }) {
  const ref = useRef<HTMLElement & { reset?: () => void }>(null)

  useEffect(() => {
    const el = ref.current
    if (!el) return
    const onStateChange = (ev: Event) => {
      const detail = (ev as CustomEvent<{ state: string; payload?: string }>).detail
      onToken(detail.state === 'verified' && detail.payload ? detail.payload : '')
    }
    el.addEventListener('statechange', onStateChange)
    return () => el.removeEventListener('statechange', onStateChange)
  }, [onToken])

  return (
    <altcha-widget
      ref={ref}
      challengeurl="/api/captcha/altcha/challenge?purpose=auth"
      auto="onload"
      hidelogo
      hidefooter
    />
  )
}
