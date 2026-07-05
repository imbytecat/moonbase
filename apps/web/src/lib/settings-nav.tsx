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
import { m } from '#paraglide/messages.js'
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
    label: m.settingsNav_general,
    items: [
      {
        path: '/settings/site',
        label: m.settingsNav_site,
        icon: <IdcardOutlined />,
        permission: Permission.SETTINGS_READ,
      },
      {
        path: '/settings/registration',
        label: m.settingsNav_registration,
        icon: <UserAddOutlined />,
        permission: Permission.SETTINGS_READ,
      },
    ],
  },
  {
    key: 'communication',
    label: m.settingsNav_communication,
    items: [
      {
        path: '/settings/email',
        label: m.systemPage_emailCard,
        icon: <MailOutlined />,
        permission: Permission.SYSTEM_READ,
      },
      {
        path: '/settings/sms',
        label: m.systemPage_smsCard,
        icon: <MessageOutlined />,
        permission: Permission.SYSTEM_READ,
      },
    ],
  },
  {
    key: 'identity',
    label: m.settingsNav_identity,
    items: [
      {
        path: '/settings/captcha',
        label: m.systemPage_captchaCard,
        icon: <SafetyOutlined />,
        permission: Permission.SYSTEM_READ,
      },
      {
        path: '/settings/oauth',
        label: m.systemPage_oauthCard,
        icon: <WechatOutlined />,
        permission: Permission.SYSTEM_READ,
      },
    ],
  },
  {
    key: 'payment',
    label: m.settingsNav_payment,
    items: [
      {
        path: '/settings/payment',
        label: m.systemPage_paymentCard,
        icon: <PayCircleOutlined />,
        permission: Permission.SYSTEM_READ,
      },
    ],
  },
  {
    key: 'infrastructure',
    label: m.settingsNav_infrastructure,
    items: [
      {
        path: '/settings/storage',
        label: m.systemPage_storageCard,
        icon: <CloudServerOutlined />,
        permission: Permission.SYSTEM_READ,
      },
      {
        path: '/settings/llm',
        label: m.systemPage_llmCard,
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
