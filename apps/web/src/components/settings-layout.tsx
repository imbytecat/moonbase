import { useSuspenseQuery } from '@connectrpc/connect-query'
import { getMe } from '@moonbase/api-client'
import { Link, Outlet, useLocation, useNavigate } from '@tanstack/react-router'
import { Card } from 'antd'
import { useMemo } from 'react'
import { SectionNavLayout } from '#components/section-nav-layout'
import { visibleSettingsGroups } from '#lib/settings-nav'

// Desktop: master-detail with a grouped side menu (the GitHub/Vercel settings
// shape — scales to any number of sections without the horizontal-Tabs
// squeeze). Mobile: the menu collapses into a Select above the content.
export function SettingsLayout() {
  const { data } = useSuspenseQuery(getMe)
  const { pathname } = useLocation()
  const navigate = useNavigate()

  const groups = useMemo(() => visibleSettingsGroups(data.user), [data.user])

  return (
    <div className="mx-auto max-w-5xl">
      <SectionNavLayout
        groups={groups.map((group) => ({
          key: group.key,
          label: group.label(),
          items: group.items.map((item) => ({
            key: item.path,
            label: item.label(),
            icon: item.icon,
            render: <Link to={item.path}>{item.label()}</Link>,
          })),
        }))}
        selectedKey={pathname}
        onSelect={(path) => void navigate({ to: path })}
      >
        <Card>
          <Outlet />
        </Card>
      </SectionNavLayout>
    </div>
  )
}
