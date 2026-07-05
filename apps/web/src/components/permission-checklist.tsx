import { SearchOutlined } from '@ant-design/icons'
import type { Permission } from '@moonbase/api-client'
import { Checkbox, Empty, Input } from 'antd'
import { useMemo, useState } from 'react'
import {
  permissionDescription,
  permissionKey,
  permissionResource,
  permissionResourceLabel,
} from '#lib/permissions'
import { m } from '#paraglide/messages.js'

export interface PermissionOption {
  readonly permission: Permission
  readonly description: string
}

// Grouped permission picker (the scalable successor of a flat checkbox list):
// permission keys are `resource.action`, so groups derive from the key prefix
// — no extra metadata needed. Each group gets a select-all checkbox
// (indeterminate when partial) and the whole list is search-filterable, which
// stays usable at dozens of resources where a flat list drowns. Acts as an
// antd Form custom control (value/onChange).
export function PermissionChecklist({
  options,
  value,
  onChange,
}: {
  options: PermissionOption[]
  value?: Permission[]
  onChange?: (next: Permission[]) => void
}) {
  const [search, setSearch] = useState('')
  const selected = useMemo(() => new Set(value ?? []), [value])

  const groups = useMemo(() => {
    const byResource = new Map<string, PermissionOption[]>()
    for (const opt of options) {
      const resource = permissionResource(opt.permission)
      const list = byResource.get(resource)
      if (list) list.push(opt)
      else byResource.set(resource, [opt])
    }
    return [...byResource.entries()].map(([resource, items]) => ({ resource, items }))
  }, [options])

  const query = search.trim().toLowerCase()
  const visibleGroups = useMemo(() => {
    if (!query) return groups
    return groups
      .map((g) => ({
        resource: g.resource,
        items:
          g.resource.includes(query) ||
          permissionResourceLabel(g.resource).toLowerCase().includes(query)
            ? g.items
            : g.items.filter(
                (item) =>
                  permissionKey(item.permission).toLowerCase().includes(query) ||
                  permissionDescription(item.permission, item.description)
                    .toLowerCase()
                    .includes(query),
              ),
      }))
      .filter((g) => g.items.length > 0)
  }, [groups, query])

  const setKeys = (permissions: Permission[], checked: boolean) => {
    const next = new Set(selected)
    for (const permission of permissions) {
      if (checked) next.add(permission)
      else next.delete(permission)
    }
    onChange?.(options.map((o) => o.permission).filter((permission) => next.has(permission)))
  }

  return (
    <div className="flex flex-col gap-3">
      <Input
        allowClear
        size="small"
        prefix={<SearchOutlined className="text-(--ant-color-text-quaternary)" />}
        placeholder={m.rolesPage_searchPermissions()}
        value={search}
        onChange={(e) => setSearch(e.target.value)}
      />
      {visibleGroups.length === 0 ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={m.rolesPage_noPermissionMatch()} />
      ) : (
        visibleGroups.map((group) => {
          const permissions = group.items.map((i) => i.permission)
          const checkedCount = permissions.filter((permission) => selected.has(permission)).length
          return (
            <div
              key={group.resource}
              className="rounded-lg border border-(--ant-color-border-secondary) p-3"
            >
              <Checkbox
                className="font-medium"
                checked={checkedCount === permissions.length}
                indeterminate={checkedCount > 0 && checkedCount < permissions.length}
                onChange={(e) => setKeys(permissions, e.target.checked)}
              >
                {permissionResourceLabel(group.resource)}
              </Checkbox>
              <div className="mt-2 flex flex-col gap-1.5 ps-6">
                {group.items.map((item) => (
                  <Checkbox
                    key={item.permission}
                    checked={selected.has(item.permission)}
                    onChange={(e) => setKeys([item.permission], e.target.checked)}
                  >
                    <code className="text-xs">{permissionKey(item.permission)}</code>
                    <span className="ms-2 text-xs text-(--ant-color-text-tertiary)">
                      {permissionDescription(item.permission, item.description)}
                    </span>
                  </Checkbox>
                ))}
              </div>
            </div>
          )
        })
      )}
    </div>
  )
}
