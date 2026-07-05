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
import { m } from '#paraglide/messages.js'
import { hasAnyPermission } from './session'
import { SETTINGS_PERMISSIONS } from './settings-nav'

// The navigation tree is the single source of truth for the sidebar: leaves
// carry a route path + optional permission; branches group leaves and vanish
// automatically when every child is filtered out. Adding a page = one entry;
// adding a section = wrapping entries in a branch. Rendering, filtering,
// selection and expansion all derive from this tree. Labels are Paraglide
// message functions, resolved at render time for the active locale.
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
  { path: '/', label: m.nav_dashboard, icon: <DashboardOutlined /> },
  {
    path: '/workflows',
    label: m.nav_workflows,
    icon: <NodeIndexOutlined />,
    permissions: [Permission.WORKFLOW_READ],
  },
  {
    path: '/payments',
    label: m.nav_payments,
    icon: <PayCircleOutlined />,
    permissions: [Permission.PAYMENT_READ],
  },
  {
    key: 'access',
    label: m.nav_access,
    icon: <SafetyCertificateOutlined />,
    children: [
      {
        path: '/users',
        label: m.nav_users,
        icon: <UserOutlined />,
        permissions: [Permission.USER_READ],
      },
      {
        path: '/roles',
        label: m.nav_roles,
        icon: <TeamOutlined />,
        permissions: [Permission.ROLE_READ],
      },
      {
        path: '/audit',
        label: m.nav_audit,
        icon: <FileSearchOutlined />,
        permissions: [Permission.AUDIT_READ],
      },
    ],
  },
  {
    path: '/settings',
    label: m.nav_settings,
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
