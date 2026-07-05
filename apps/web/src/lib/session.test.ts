import { create } from '@bufbuild/protobuf'
import { Code, ConnectError, createClient, createRouterTransport } from '@connectrpc/connect'
import {
  AuthService,
  CurrentUserSchema,
  GetMeResponseSchema,
  LoginResponseSchema,
  Permission,
} from '@moonbase/api-client'
import { QueryClient } from '@tanstack/react-query'
import { describe, expect, it } from 'vitest'
import { ensureSession, hasPermission } from './session'

const adminUser = create(CurrentUserSchema, {
  id: '0b7acb98-1a2f-4d3c-9d67-2f6ad8f0a111',
  email: 'admin@example.com',
  name: 'Admin',
  permissions: [Permission.ALL],
})

function newBackend(opts: { authenticated: boolean }) {
  return createRouterTransport(({ service }) => {
    service(AuthService, {
      getMe: () => {
        if (!opts.authenticated) {
          throw new ConnectError('authentication required', Code.Unauthenticated)
        }
        return create(GetMeResponseSchema, { user: adminUser })
      },
      login: () => create(LoginResponseSchema, { user: adminUser }),
    })
  })
}

describe('session guard contract', () => {
  it('resolves the current user when authenticated', async () => {
    const transport = newBackend({ authenticated: true })
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })

    const user = await ensureSession(queryClient, transport)

    expect(user.email).toBe('admin@example.com')
  })

  it('rejects with unauthenticated when there is no session', async () => {
    const transport = newBackend({ authenticated: false })
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })

    await expect(ensureSession(queryClient, transport)).rejects.toSatisfy(
      (err) => err instanceof ConnectError && err.code === Code.Unauthenticated,
    )
  })

  it('login RPC returns the session user', async () => {
    const transport = newBackend({ authenticated: false })
    const client = createClient(AuthService, transport)

    const res = await client.login({ identifier: 'admin@example.com', password: 'password123' })

    expect(res.user?.permissions).toContain(Permission.ALL)
  })
})

describe('hasPermission', () => {
  it('wildcard grants everything', () => {
    expect(hasPermission(adminUser, Permission.SETTINGS_WRITE)).toBe(true)
  })

  it('missing permission denies', () => {
    const user = create(CurrentUserSchema, { permissions: [Permission.REPORT_READ] })
    expect(hasPermission(user, Permission.USER_WRITE)).toBe(false)
    expect(hasPermission(user, Permission.REPORT_READ)).toBe(true)
  })

  it('undefined user denies', () => {
    expect(hasPermission(undefined, Permission.REPORT_READ)).toBe(false)
  })
})
