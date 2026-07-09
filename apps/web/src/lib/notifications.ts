interface CategoryMeta {
  label: () => string
  color: string
}

const CATEGORY_META: Record<string, CategoryMeta> = {
  system: { label: () => '系统', color: 'blue' },
  security: { label: () => '安全', color: 'red' },
  account: { label: () => '账号', color: 'green' },
  payment: { label: () => '支付', color: 'gold' },
  workflow: { label: () => '工作流', color: 'purple' },
}

export function notificationCategory(category: string): { label: string; color: string } {
  const meta = CATEGORY_META[category]
  return { label: meta ? meta.label() : category, color: meta?.color ?? 'default' }
}
