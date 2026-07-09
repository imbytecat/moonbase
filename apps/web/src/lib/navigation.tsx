import {
  DashboardOutlined,
  FileSearchOutlined,
  NodeIndexOutlined,
  PayCircleOutlined,
  SafetyCertificateOutlined,
  SettingOutlined,
  TeamOutlined,
  UserOutlined,
} from '@ant-design/icons'
import type { CurrentUser } from '@moonbase/api-client'
import { Permission } from '@moonbase/api-client'
import type { MenuProps } from 'antd'
import type { ReactNode } from 'react'
import { hasAnyPermission } from './session'
import { SETTINGS_PERMISSIONS } from './settings-nav'

export type NavNode = NavLeaf | NavBranch

interface NavLeaf {
  path: string
  label: () => string
  icon: ReactNode
  permissions?: readonly Permission[]
}

interface NavBranch {
  key: string
  label: () => string
  icon: ReactNode
  children: NavNode[]
}

export const NAV_TREE: NavNode[] = [
  { path: '/', label: () => '仪表盘', icon: <DashboardOutlined /> },
  {
    path: '/workflows',
    label: () => '工作流',
    icon: <NodeIndexOutlined />,
    permissions: [Permission.WORKFLOW_READ],
  },
  {
    path: '/payments',
    label: () => '支付订单',
    icon: <PayCircleOutlined />,
    permissions: [Permission.PAYMENT_READ],
  },
  {
    key: 'access',
    label: () => '权限管理',
    icon: <SafetyCertificateOutlined />,
    children: [
      {
        path: '/users',
        label: () => '用户管理',
        icon: <UserOutlined />,
        permissions: [Permission.USER_READ],
      },
      {
        path: '/roles',
        label: () => '角色管理',
        icon: <TeamOutlined />,
        permissions: [Permission.ROLE_READ],
      },
      {
        path: '/audit',
        label: () => '审计日志',
        icon: <FileSearchOutlined />,
        permissions: [Permission.AUDIT_READ],
      },
    ],
  },
  {
    path: '/settings',
    label: () => '设置',
    icon: <SettingOutlined />,
    permissions: SETTINGS_PERMISSIONS,
  },
]

type MenuItem = NonNullable<MenuProps['items']>[number]

interface BuildOptions {
  user: CurrentUser | undefined
  renderLink: (path: string, label: string) => ReactNode
}

export function buildMenuItems(nodes: NavNode[], opts: BuildOptions): MenuItem[] {
  const items: MenuItem[] = []
  for (const node of nodes) {
    if ('children' in node) {
      const children = buildMenuItems(node.children, opts)
      if (children.length === 0) continue
      items.push({
        key: node.key,
        icon: node.icon,
        label: node.label(),
        children,
      })
      continue
    }
    if (node.permissions && !hasAnyPermission(opts.user, node.permissions)) continue
    items.push({
      key: node.path,
      icon: node.icon,
      label: opts.renderLink(node.path, node.label()),
    })
  }
  return items
}

// Longest-prefix match so /users/123 still highlights /users; returns the
// branch keys on the way down so the submenu is open after a full reload.
export function navStateForPath(pathname: string): { selectedKey: string; openKeys: string[] } {
  let best: { path: string; parents: string[] } | undefined

  const walk = (nodes: NavNode[], parents: string[]) => {
    for (const node of nodes) {
      if ('children' in node) {
        walk(node.children, [...parents, node.key])
        continue
      }
      const matches = node.path === '/' ? pathname === '/' : pathname.startsWith(node.path)
      if (matches && (!best || node.path.length > best.path.length)) {
        best = { path: node.path, parents }
      }
    }
  }
  walk(NAV_TREE, [])

  return { selectedKey: best?.path ?? '', openKeys: best?.parents ?? [] }
}
