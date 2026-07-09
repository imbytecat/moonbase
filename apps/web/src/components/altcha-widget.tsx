import 'altcha'
import 'altcha/i18n/zh-cn'
import { useEffect, useRef } from 'react'

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
