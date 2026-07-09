import { Permission } from '@moonbase/api-client'

// Derive a Permission enum's wire/DB key (mirrors the backend): ALL is the "*"
// wildcard, otherwise the member name lower-cased with its last underscore
// turned into a dot (USER_READ -> "user.read"). The proto enum is the source of
// truth; this bridges to the string-keyed Chinese copy catalog.
export function permissionKey(p: Permission): string {
  if (p === Permission.ALL) return '*'
  const name = Permission[p]
  if (!name) return ''
  const body = name.toLowerCase()
  const dot = body.lastIndexOf('_')
  return dot >= 0 ? `${body.slice(0, dot)}.${body.slice(dot + 1)}` : body
}

// Client-side Chinese copy for the permission catalog. The wire description
// stays the English source of truth; a missing key falls back to it.
export const PERMISSION_DESCRIPTIONS: Record<string, () => string> = {
  'report.read': () => '查看报表与统计数据',
  'user.read': () => '查看用户及其角色',
  'user.write': () => '创建、修改和删除用户，分配角色',
  'role.read': () => '查看角色与权限',
  'role.write': () => '创建、修改和删除角色',
  'settings.read': () => '查看业务设置',
  'settings.write': () => '修改业务设置',
  'system.read': () => '查看系统设置（存储、验证码、邮件、短信、AI、支付）',
  'system.write': () => '修改系统设置并执行通道测试',
  'workflow.read': () => '查看工作流运行及其步骤',
  'workflow.write': () => '取消、恢复和触发工作流',
  'audit.read': () => '查看管理操作审计日志',
  'payment.read': () => '查看支付订单',
  'payment.write': () => '创建、同步和退款支付订单',
}

export function permissionDescription(p: Permission, fallback: string): string {
  return PERMISSION_DESCRIPTIONS[permissionKey(p)]?.() ?? fallback
}

export function permissionResource(p: Permission): string {
  return permissionKey(p).split('.')[0] ?? ''
}

// Group headers for the permission picker, keyed by the `resource` prefix of a
// `resource.action` key. Unknown resources fall back to the raw prefix.
export const RESOURCE_LABELS: Record<string, () => string> = {
  report: () => '数据报表',
  user: () => '用户管理',
  role: () => '角色管理',
  settings: () => '业务设置',
  system: () => '系统设置',
  workflow: () => '工作流',
  audit: () => '审计日志',
  payment: () => '支付',
}

export function permissionResourceLabel(resource: string): string {
  return RESOURCE_LABELS[resource]?.() ?? resource
}
