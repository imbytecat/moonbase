import { Permission } from '@moonbase/api-client'
import { describe, expect, it } from 'vitest'
import {
  PERMISSION_DESCRIPTIONS,
  permissionKey,
  permissionResource,
  RESOURCE_LABELS,
} from '#lib/permissions'

// Drift-gate: the hand-written permission catalog (permissions.ts) mirrors the
// proto Permission enum — the single source of truth. Adding a permission on the
// backend without a label here would otherwise fall back silently; this turns it
// into a failed build instead.

// Grantable permissions are every enum member except UNSPECIFIED and the admin
// wildcard ALL, which carry no catalog label.
const grantable = (Object.values(Permission).filter((v) => typeof v === 'number') as Permission[])
  .filter((permission) => permission !== Permission.UNSPECIFIED && permission !== Permission.ALL)
  .map((permission) => ({ permission, key: permissionKey(permission) }))

describe('permission catalog mirror tracks the proto Permission enum', () => {
  it('PERMISSION_DESCRIPTIONS keys exactly the grantable permissions', () => {
    const wanted = [...new Set(grantable.map(({ key }) => key))].sort()
    const have = Object.keys(PERMISSION_DESCRIPTIONS).sort()
    expect(have).toEqual(wanted)
  })

  it('RESOURCE_LABELS keys exactly the permission resources', () => {
    const wanted = [
      ...new Set(grantable.map(({ permission }) => permissionResource(permission))),
    ].sort()
    const have = Object.keys(RESOURCE_LABELS).sort()
    expect(have).toEqual(wanted)
  })
})
