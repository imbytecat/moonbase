import { StyleProvider } from '@ant-design/cssinjs'
import { App as AntApp, ConfigProvider, theme } from 'antd'
import type { ReactNode } from 'react'
import { ANTD_LOCALE } from '#lib/locale'
import { ThemeModeProvider, useThemeMode } from './theme-mode'

interface AppProvidersProps {
  children: ReactNode
}

/**
 * Wires Ant Design so it coexists with Tailwind v4:
 * - `StyleProvider layer` emits antd styles into the `antd` CSS layer.
 * - The layer order is declared in styles.css.
 * - `ConfigProvider` must be nested inside `StyleProvider` so icon styles also
 *   pick up the layer. `AntApp` enables the static message/notification APIs.
 * - antd locale is fixed to Simplified Chinese.
 * - Light/dark follows ThemeModeProvider, which also toggles `.dark` on
 *   <html> so Tailwind's dark: variant stays in sync with antd tokens.
 */
export function AppProviders({ children }: AppProvidersProps) {
  return (
    <ThemeModeProvider>
      <AntThemeBridge>{children}</AntThemeBridge>
    </ThemeModeProvider>
  )
}

const FONT_FAMILY = [
  '-apple-system',
  'BlinkMacSystemFont',
  "'Segoe UI'",
  'Roboto',
  "'Helvetica Neue'",
  "'PingFang SC'",
  "'Hiragino Sans GB'",
  "'Microsoft YaHei'",
  'sans-serif',
].join(', ')

function AntThemeBridge({ children }: AppProvidersProps) {
  const { resolved } = useThemeMode()
  const dark = resolved === 'dark'

  return (
    <StyleProvider layer>
      <ConfigProvider
        locale={ANTD_LOCALE}
        theme={{
          algorithm: dark ? theme.darkAlgorithm : theme.defaultAlgorithm,
          token: {
            colorPrimary: '#4f6ef7',
            colorInfo: '#4f6ef7',
            borderRadius: 8,
            fontFamily: FONT_FAMILY,
            colorBgLayout: dark ? '#0d0d11' : '#f0f1f5',
            ...(dark ? { colorBgContainer: '#17171c', colorBgElevated: '#1f1f26' } : {}),
          },
          components: {
            Layout: {
              headerBg: dark ? '#17171c' : '#ffffff',
              // One dark brand rail for the nav in BOTH themes (and both the
              // desktop Sider and the mobile Drawer render it), so navigation
              // reads identically everywhere while content stays theme-driven.
              siderBg: '#161a26',
              headerHeight: 56,
            },
            Menu: {
              itemBorderRadius: 8,
              itemMarginInline: 8,
              itemMarginBlock: 4,
              subMenuItemBorderRadius: 8,
              activeBarBorderWidth: 0,
              darkItemBg: '#161a26',
              darkSubMenuItemBg: '#11141d',
              darkItemSelectedBg: '#4f6ef7',
              darkPopupBg: '#161a26',
            },
            Card: {
              boxShadowTertiary: dark
                ? '0 1px 2px rgb(0 0 0 / 0.5)'
                : '0 1px 2px rgb(16 24 40 / 0.06), 0 1px 6px -1px rgb(16 24 40 / 0.04)',
            },
            Table: { headerBg: 'transparent' },
          },
        }}
      >
        <AntApp>{children}</AntApp>
      </ConfigProvider>
    </StyleProvider>
  )
}
