import { Permission } from '@moonbase/api-client'
import { m } from '#paraglide/messages.js'

// Derive a Permission enum's wire/DB key (mirrors the backend): ALL is the "*"
// wildcard, otherwise the member name lower-cased with its last underscore
// turned into a dot (USER_READ -> "user.read"). The proto enum is the source of
// truth; this bridges to the string-keyed i18n catalog.
export function permissionKey(p: Permission): string {
  if (p === Permission.ALL) return '*'
  const name = Permission[p]
  if (!name) return ''
  const body = name.toLowerCase()
  const dot = body.lastIndexOf('_')
  return dot >= 0 ? `${body.slice(0, dot)}.${body.slice(dot + 1)}` : body
}

// Client-side translations for the permission catalog. The wire description
// stays the English source of truth; an untranslated key falls back to it.
const PERMISSION_DESCRIPTIONS: Record<string, () => string> = {
  'report.read': m.permission_report_read,
  'user.read': m.permission_user_read,
  'user.write': m.permission_user_write,
  'role.read': m.permission_role_read,
  'role.write': m.permission_role_write,
  'settings.read': m.permission_settings_read,
  'settings.write': m.permission_settings_write,
  'system.read': m.permission_system_read,
  'system.write': m.permission_system_write,
  'workflow.read': m.permission_workflow_read,
  'workflow.write': m.permission_workflow_write,
  'audit.read': m.permission_audit_read,
  'payment.read': m.permission_payment_read,
  'payment.write': m.permission_payment_write,
}

export function permissionDescription(p: Permission, fallback: string): string {
  return PERMISSION_DESCRIPTIONS[permissionKey(p)]?.() ?? fallback
}

export function permissionResource(p: Permission): string {
  return permissionKey(p).split('.')[0] ?? ''
}

// Group headers for the permission picker, keyed by the `resource` prefix of a
// `resource.action` key. Unknown resources fall back to the raw prefix.
const RESOURCE_LABELS: Record<string, () => string> = {
  report: m.permissionGroup_report,
  user: m.permissionGroup_user,
  role: m.permissionGroup_role,
  settings: m.permissionGroup_settings,
  system: m.permissionGroup_system,
  workflow: m.permissionGroup_workflow,
  audit: m.permissionGroup_audit,
  payment: m.permissionGroup_payment,
}

export function permissionResourceLabel(resource: string): string {
  return RESOURCE_LABELS[resource]?.() ?? resource
}
