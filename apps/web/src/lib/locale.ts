import type { Locale as AntdLocale } from 'antd/es/locale'
import enUS from 'antd/locale/en_US'
import zhCN from 'antd/locale/zh_CN'
import type { Locale } from '#paraglide/runtime.js'

// Native-language names so each option is readable in its own locale.
export const LOCALE_LABELS: Record<Locale, string> = {
  'zh-CN': '简体中文',
  en: 'English',
}

// antd's built-in strings (pagination, modals, pickers) follow the app locale.
export const ANTD_LOCALES: Record<Locale, AntdLocale> = {
  'zh-CN': zhCN,
  en: enUS,
}
