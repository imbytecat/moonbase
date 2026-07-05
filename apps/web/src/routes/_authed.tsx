import {
  CloseOutlined,
  GlobalOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuOutlined,
  MenuUnfoldOutlined,
  MoonOutlined,
  SunOutlined,
  UserOutlined,
} from '@ant-design/icons'
import { useMutation, useQuery, useSuspenseQuery } from '@connectrpc/connect-query'
import { getMe, getSiteInfo, logout, updateProfile } from '@moonbase/api-client'
import { useQueryClient } from '@tanstack/react-query'
import {
  createFileRoute,
  Link,
  Outlet,
  redirect,
  useLocation,
  useRouter,
} from '@tanstack/react-router'
import { Avatar, Button, Drawer, Dropdown, Layout, Menu } from 'antd'
import { useEffect, useMemo, useState } from 'react'
import { NotificationBell } from '#components/notification-bell'
import { LOCALE_LABELS } from '#lib/locale'
import { buildMenuItems, NAV_TREE, navStateForPath } from '#lib/navigation'
import { sessionQueryOptions } from '#lib/session'
import { siteName } from '#lib/site'
import { m } from '#paraglide/messages.js'
import { getLocale, isLocale, locales, setLocale } from '#paraglide/runtime.js'
import { type ThemeMode, useThemeMode } from '#providers/theme-mode'

export const Route = createFileRoute('/_authed')({
  // The guard: resolve the session before any child route loads. Redirect to
  // /login (remembering where we were) on unauthenticated.
  beforeLoad: async ({ context: { queryClient, transport }, location }) => {
    try {
      await queryClient.ensureQueryData(sessionQueryOptions(transport))
    } catch {
      throw redirect({ to: '/login', search: { redirect: location.href } })
    }
  },
  component: AuthedLayout,
})

const THEME_LABELS: Record<ThemeMode, () => string> = {
  light: m.theme_light,
  dark: m.theme_dark,
  system: m.theme_system,
}

function AuthedLayout() {
  const router = useRouter()
  const location = useLocation()
  const queryClient = useQueryClient()
  const { data } = useSuspenseQuery(getMe)
  const { data: siteInfo } = useQuery(getSiteInfo)
  const { mode, setMode } = useThemeMode()
  const user = data.user

  const updateLocale = useMutation(updateProfile)
  // The account's stored locale wins over browser detection once the session
  // resolves, so language follows the user across devices. setLocale reloads
  // once; the reloaded page reads the same locale, so it converges (no loop).
  useEffect(() => {
    const stored = user?.locale
    if (stored && isLocale(stored) && stored !== getLocale()) {
      setLocale(stored)
    }
  }, [user?.locale])

  const logoutMutation = useMutation(logout, {
    onSettled: async () => {
      queryClient.clear()
      await router.navigate({ to: '/login' })
    },
  })

  // Permission-filtered tree → antd items; branches with no visible children
  // disappear entirely. Backend authz still enforces every RPC — this is UX.
  const menuItems = useMemo(
    () =>
      buildMenuItems(NAV_TREE, {
        user,
        renderLink: (path, label) => <Link to={path}>{label}</Link>,
      }),
    [user],
  )

  const { selectedKey, openKeys: routeOpenKeys } = navStateForPath(location.pathname)
  // Auto-open the submenu of the active route while keeping manual open/close
  // interactive (uncontrolled after the first render per navigation).
  const [openKeys, setOpenKeys] = useState<string[]>(routeOpenKeys)
  const [lastRouteKeys, setLastRouteKeys] = useState(routeOpenKeys.join())
  if (routeOpenKeys.join() !== lastRouteKeys) {
    setLastRouteKeys(routeOpenKeys.join())
    setOpenKeys((prev) => [...new Set([...prev, ...routeOpenKeys])])
  }

  const [navOpen, setNavOpen] = useState(false)
  const [lastPathname, setLastPathname] = useState(location.pathname)
  if (location.pathname !== lastPathname) {
    setLastPathname(location.pathname)
    setNavOpen(false)
  }

  const [collapsed, setCollapsed] = useState(() => localStorage.getItem('sidebarCollapsed') === '1')
  const toggleCollapsed = () => {
    setCollapsed((prev) => {
      localStorage.setItem('sidebarCollapsed', prev ? '0' : '1')
      return !prev
    })
  }

  const brand = (
    <div className="flex items-center gap-2.5">
      {siteInfo?.logoUrl ? (
        <img src={siteInfo.logoUrl} alt="" className="size-7 shrink-0 object-contain" />
      ) : (
        <div className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-(--ant-color-primary) text-sm font-bold text-white">
          {siteName(siteInfo).charAt(0).toUpperCase()}
        </div>
      )}
      <span className="truncate text-[15px] font-semibold text-white/90">{siteName(siteInfo)}</span>
    </div>
  )

  const navMenu = (
    <Menu
      mode="inline"
      theme="dark"
      selectedKeys={[selectedKey]}
      openKeys={openKeys}
      onOpenChange={setOpenKeys}
      items={menuItems}
      className="border-e-0"
    />
  )

  return (
    <Layout className="min-h-screen">
      <Layout.Sider
        theme="dark"
        width={224}
        collapsible
        collapsed={collapsed}
        collapsedWidth={64}
        trigger={null}
        className="sticky top-0 !h-screen overflow-y-auto max-lg:!hidden"
      >
        <div className={`flex h-14 items-center ${collapsed ? 'justify-center' : 'px-5'}`}>
          {collapsed ? (
            siteInfo?.logoUrl ? (
              <img src={siteInfo.logoUrl} alt="" className="size-7 object-contain" />
            ) : (
              <div className="flex size-7 items-center justify-center rounded-lg bg-(--ant-color-primary) text-sm font-bold text-white">
                {siteName(siteInfo).charAt(0).toUpperCase()}
              </div>
            )
          ) : (
            brand
          )}
        </div>
        {collapsed ? (
          <Menu
            mode="inline"
            theme="dark"
            selectedKeys={[selectedKey]}
            items={menuItems}
            className="border-e-0"
          />
        ) : (
          navMenu
        )}
      </Layout.Sider>

      <Drawer
        open={navOpen}
        onClose={() => setNavOpen(false)}
        placement="left"
        size={260}
        title={brand}
        closeIcon={<CloseOutlined className="!text-white/70" />}
        styles={{
          header: { background: '#161a26', borderBottom: '1px solid rgb(255 255 255 / 0.08)' },
          body: { padding: 0, background: '#161a26' },
        }}
        rootClassName="lg:hidden"
      >
        {navMenu}
      </Drawer>

      <Layout>
        <Layout.Header className="sticky top-0 z-10 flex items-center justify-between gap-3 px-4 shadow-xs md:px-6">
          <Button
            type="text"
            icon={<MenuOutlined />}
            aria-label={m.nav_openMenu()}
            onClick={() => setNavOpen(true)}
            className="lg:!hidden"
          />
          <Button
            type="text"
            icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
            aria-label={m.nav_toggleSidebar()}
            onClick={toggleCollapsed}
            className="max-lg:!hidden"
          />
          <div className="flex items-center gap-1">
            <NotificationBell />
            <Dropdown
              popupRender={(menu) => (
                <div className="min-w-56 rounded-lg bg-(--ant-color-bg-elevated) shadow-(--ant-box-shadow-secondary)">
                  <div className="flex items-center gap-3 px-4 py-3">
                    <Avatar size={40} src={user?.avatarUrl || undefined}>
                      {user?.name?.charAt(0).toUpperCase()}
                    </Avatar>
                    <div className="min-w-0">
                      <div className="truncate text-sm font-medium">{user?.name}</div>
                      <div className="truncate text-xs text-(--ant-color-text-tertiary)">
                        {user?.email || user?.username || user?.phone}
                      </div>
                    </div>
                  </div>
                  <div className="border-t border-(--ant-color-split) [&_.ant-dropdown-menu]:!shadow-none">
                    {menu}
                  </div>
                </div>
              )}
              menu={{
                selectable: false,
                items: [
                  {
                    key: 'profile',
                    icon: <UserOutlined />,
                    label: <Link to="/profile">{m.nav_profile()}</Link>,
                  },
                  {
                    key: 'theme',
                    icon: mode === 'dark' ? <MoonOutlined /> : <SunOutlined />,
                    label: m.nav_theme(),
                    children: (['light', 'dark', 'system'] as const).map((value) => ({
                      key: `theme:${value}`,
                      label: THEME_LABELS[value](),
                      disabled: mode === value,
                      onClick: () => setMode(value),
                    })),
                  },
                  // The language switcher only exists in multi-locale builds —
                  // with one supported locale there is nothing to switch.
                  ...(locales.length > 1
                    ? [
                        {
                          key: 'language',
                          icon: <GlobalOutlined />,
                          label: m.nav_language(),
                          children: locales.map((locale) => ({
                            key: `locale:${locale}`,
                            label: LOCALE_LABELS[locale],
                            disabled: getLocale() === locale,
                            onClick: () =>
                              updateLocale.mutate(
                                { locale },
                                { onSuccess: () => setLocale(locale) },
                              ),
                          })),
                        },
                      ]
                    : []),
                  { type: 'divider' as const },
                  {
                    key: 'logout',
                    icon: <LogoutOutlined />,
                    danger: true,
                    label: m.nav_signOut(),
                    onClick: () => logoutMutation.mutate({}),
                  },
                ],
              }}
              trigger={['click']}
            >
              <button
                type="button"
                className="flex cursor-pointer items-center gap-2 rounded-lg border-0 bg-transparent px-2 py-1.5 transition-colors hover:bg-black/5 dark:hover:bg-white/10"
              >
                <Avatar src={user?.avatarUrl || undefined} size="small">
                  {user?.name?.charAt(0).toUpperCase()}
                </Avatar>
                <span className="text-sm">{user?.name}</span>
              </button>
            </Dropdown>
          </div>
        </Layout.Header>

        <Layout.Content className="p-4 md:p-6">
          <Outlet />
        </Layout.Content>

        {siteInfo?.copyright || siteInfo?.icpBeian ? (
          <Layout.Footer className="py-4 text-center">
            <div className="space-x-3 text-xs text-(--ant-color-text-quaternary)">
              {siteInfo.copyright ? <span>{siteInfo.copyright}</span> : null}
              {siteInfo.icpBeian ? (
                <a
                  href="https://beian.miit.gov.cn/"
                  target="_blank"
                  rel="noreferrer"
                  className="text-(--ant-color-text-quaternary) hover:text-(--ant-color-text-secondary)"
                >
                  {siteInfo.icpBeian}
                </a>
              ) : null}
            </div>
          </Layout.Footer>
        ) : null}
      </Layout>
    </Layout>
  )
}
