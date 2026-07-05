import { Menu, type MenuProps, Select, type SelectProps } from 'antd'
import type { ReactNode } from 'react'

export interface SectionNavItem {
  key: string
  label: string
  icon?: ReactNode
  // Menu-only richer label (e.g. a router <Link> so hover preloading works);
  // the mobile Select always uses the plain string label.
  render?: ReactNode
}

export interface SectionNavGroup {
  key: string
  label?: string
  items: SectionNavItem[]
}

// Master-detail shell shared by every sectioned page (settings, profile):
// desktop renders a side menu, mobile collapses it into a Select above the
// content. Callers own what selection means (route navigation vs local tab).
export function SectionNavLayout({
  groups,
  selectedKey,
  onSelect,
  children,
}: {
  groups: SectionNavGroup[]
  selectedKey: string
  onSelect: (key: string) => void
  children: ReactNode
}) {
  const menuItems: NonNullable<MenuProps['items']> = groups.flatMap(
    (group): NonNullable<MenuProps['items']> =>
      group.label
        ? [
            {
              key: group.key,
              type: 'group' as const,
              label: group.label,
              children: group.items.map((item) => ({
                key: item.key,
                icon: item.icon,
                label: item.render ?? item.label,
              })),
            },
          ]
        : group.items.map((item) => ({
            key: item.key,
            icon: item.icon,
            label: item.render ?? item.label,
          })),
  )

  const selectOptions: NonNullable<SelectProps['options']> = groups.flatMap(
    (group): NonNullable<SelectProps['options']> =>
      group.label
        ? [
            {
              label: group.label,
              options: group.items.map((item) => ({ label: item.label, value: item.key })),
            },
          ]
        : group.items.map((item) => ({ label: item.label, value: item.key })),
  )

  return (
    <div>
      <div className="mb-4 md:hidden">
        <Select
          className="w-full"
          value={selectedKey}
          options={selectOptions}
          onChange={onSelect}
        />
      </div>
      <div className="flex gap-6">
        <Menu
          mode="inline"
          className="!hidden w-52 shrink-0 self-start rounded-lg border-(--ant-color-border) md:!block"
          selectedKeys={[selectedKey]}
          items={menuItems}
          onClick={({ key }) => onSelect(key)}
        />
        <div className="min-w-0 flex-1">{children}</div>
      </div>
    </div>
  )
}
