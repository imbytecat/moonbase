import { QuestionCircleOutlined } from '@ant-design/icons'
import type { ComponentType, CSSProperties } from 'react'
import { lazy, Suspense } from 'react'

interface IconProps {
  className?: string
  style?: CSSProperties
}

const iconCache = new Map<string, ComponentType<IconProps>>()

function iconFor(ref: string): ComponentType<IconProps> {
  const cached = iconCache.get(ref)
  if (cached) return cached

  const [namespace, name] = ref.split(':', 2)
  if (namespace !== 'antd' || !name) return QuestionCircleOutlined

  const icon = lazy(async () => {
    const icons = (await import('@ant-design/icons')) as unknown as Record<
      string,
      ComponentType<IconProps>
    >
    return { default: icons[name] ?? QuestionCircleOutlined }
  })
  iconCache.set(ref, icon)
  return icon
}

export function ProviderIcon({
  iconRef,
  color,
  className = 'text-lg',
}: {
  iconRef: string
  color?: string
  className?: string
}) {
  const Icon = iconFor(iconRef)
  const fallback = <QuestionCircleOutlined className={className} style={{ color }} />
  return (
    <Suspense fallback={fallback}>
      <Icon className={className} style={{ color }} />
    </Suspense>
  )
}
