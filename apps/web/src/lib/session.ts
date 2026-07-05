import type { Transport } from '@connectrpc/connect'
import { createConnectQueryKey, createQueryOptions } from '@connectrpc/connect-query'
import { type CurrentUser, getMe, Permission } from '@moonbase/api-client'
import type { QueryClient } from '@tanstack/react-query'
import { permissionKey } from '#lib/permissions'

// Session state is just the GetMe query — no extra store. Route guards call
// ensureSession in beforeLoad; components read it with useSuspenseQuery(getMe)
// and derive permission checks from user.permissions.

export function sessionQueryOptions(transport: Transport) {
  return createQueryOptions(getMe, undefined, { transport })
}

export function sessionQueryKey() {
  return createConnectQueryKey({ schema: getMe, cardinality: 'finite' })
}

// ConnectRPC surfaces auth failures as errors with code unauthenticated.
// beforeLoad turns that into a redirect; anything else propagates.
export async function ensureSession(
  queryClient: QueryClient,
  transport: Transport,
): Promise<CurrentUser> {
  const data = await queryClient.ensureQueryData(sessionQueryOptions(transport))
  if (!data.user) throw new Error('empty session response')
  return data.user
}

export function hasPermission(user: CurrentUser | undefined, permission: Permission): boolean {
  if (!user) return false
  return user.permissions.includes(Permission.ALL) || user.permissions.includes(permission)
}

export function hasAnyPermission(
  user: CurrentUser | undefined,
  permissions: readonly Permission[],
): boolean {
  return permissions.some((permission) => hasPermission(user, permission))
}

// Thrown by route guards; RouteError renders it as a 403 page instead of the
// generic error fallback.
export class ForbiddenError extends Error {
  constructor(readonly permission: string) {
    super(`missing permission: ${permission}`)
    this.name = 'ForbiddenError'
  }
}

// beforeLoad guard for permissioned routes. The _authed layout already
// guaranteed a session, so this only turns "logged in but not allowed" into a
// clean 403 — the backend still enforces the RPCs regardless.
export async function requirePermission(
  queryClient: QueryClient,
  transport: Transport,
  permission: Permission,
): Promise<void> {
  const user = await ensureSession(queryClient, transport)
  if (!hasPermission(user, permission)) throw new ForbiddenError(permissionKey(permission))
}

// Same guard for routes visible to holders of ANY of the given permissions
// (e.g. the settings layout, shared by business and infrastructure admins).
export async function requireAnyPermission(
  queryClient: QueryClient,
  transport: Transport,
  permissions: readonly Permission[],
): Promise<void> {
  const user = await ensureSession(queryClient, transport)
  if (!hasAnyPermission(user, permissions)) {
    throw new ForbiddenError(permissions.map(permissionKey).join(' | '))
  }
}

export async function clearSession(queryClient: QueryClient) {
  // Wipe every cached query, not just getMe: cached lists/settings belong to
  // the previous account and must not flash for the next login.
  queryClient.clear()
}
