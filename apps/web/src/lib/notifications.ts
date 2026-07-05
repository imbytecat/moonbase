import { m } from '#paraglide/messages.js'

interface CategoryMeta {
  label: () => string
  color: string
}

const CATEGORY_META: Record<string, CategoryMeta> = {
  system: { label: m.notifications_categorySystem, color: 'blue' },
  security: { label: m.notifications_categorySecurity, color: 'red' },
  account: { label: m.notifications_categoryAccount, color: 'green' },
  payment: { label: m.notifications_categoryPayment, color: 'gold' },
  workflow: { label: m.notifications_categoryWorkflow, color: 'purple' },
}

export function notificationCategory(category: string): { label: string; color: string } {
  const meta = CATEGORY_META[category]
  return { label: meta ? meta.label() : category, color: meta?.color ?? 'default' }
}
