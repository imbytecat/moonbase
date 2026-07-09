import {
  CloudServerOutlined,
  IdcardOutlined,
  MailOutlined,
  MessageOutlined,
  PayCircleOutlined,
  RobotOutlined,
  SafetyOutlined,
  UserAddOutlined,
  WechatOutlined,
} from '@ant-design/icons'
import type { CurrentUser } from '@moonbase/api-client'
import { Permission } from '@moonbase/api-client'
import type { ReactNode } from 'react'
import { hasPermission } from './session'

// The settings area is ONE navigation surface over TWO backend domains:
// settings.v1 (business, settings.* perms) and system.v1 (infrastructure
// channels, system.* perms). Grouping is presentation; permissions decide
// which groups a given admin actually sees.
export interface SettingsItem {
  path: string
  label: () => string
  icon: ReactNode
  permission: Permission
}

export interface SettingsGroup {
  key: string
  label: () => string
  items: SettingsItem[]
}

export const SETTINGS_GROUPS: SettingsGroup[] = [
  {
    key: 'general',
    label: () => '通用',
    items: [
      {
        path: '/settings/site',
        label: () => '站点信息',
        icon: <IdcardOutlined />,
        permission: Permission.SETTINGS_READ,
      },
      {
        path: '/settings/registration',
        label: () => '账号与注册',
        icon: <UserAddOutlined />,
        permission: Permission.SETTINGS_READ,
      },
    ],
  },
  {
    key: 'communication',
    label: () => '通讯渠道',
    items: [
      {
        path: '/settings/email',
        label: () => '邮件服务',
        icon: <MailOutlined />,
        permission: Permission.SYSTEM_READ,
      },
      {
        path: '/settings/sms',
        label: () => '短信服务',
        icon: <MessageOutlined />,
        permission: Permission.SYSTEM_READ,
      },
    ],
  },
  {
    key: 'identity',
    label: () => '身份与安全',
    items: [
      {
        path: '/settings/captcha',
        label: () => '人机验证',
        icon: <SafetyOutlined />,
        permission: Permission.SYSTEM_READ,
      },
      {
        path: '/settings/oauth',
        label: () => '第三方登录',
        icon: <WechatOutlined />,
        permission: Permission.SYSTEM_READ,
      },
    ],
  },
  {
    key: 'payment',
    label: () => '支付',
    items: [
      {
        path: '/settings/payment',
        label: () => '支付渠道',
        icon: <PayCircleOutlined />,
        permission: Permission.SYSTEM_READ,
      },
    ],
  },
  {
    key: 'infrastructure',
    label: () => '基础设施',
    items: [
      {
        path: '/settings/storage',
        label: () => '文件存储',
        icon: <CloudServerOutlined />,
        permission: Permission.SYSTEM_READ,
      },
      {
        path: '/settings/llm',
        label: () => 'AI 模型',
        icon: <RobotOutlined />,
        permission: Permission.SYSTEM_READ,
      },
    ],
  },
]

export const SETTINGS_PERMISSIONS = [Permission.SETTINGS_READ, Permission.SYSTEM_READ] as const

export function visibleSettingsGroups(user: CurrentUser | undefined): SettingsGroup[] {
  return SETTINGS_GROUPS.map((group) => ({
    ...group,
    items: group.items.filter((item) => hasPermission(user, item.permission)),
  })).filter((group) => group.items.length > 0)
}

export function firstSettingsPath(user: CurrentUser | undefined): string | undefined {
  return visibleSettingsGroups(user)[0]?.items[0]?.path
}
