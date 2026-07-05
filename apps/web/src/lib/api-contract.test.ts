import { create } from '@bufbuild/protobuf'
import { createClient, createRouterTransport } from '@connectrpc/connect'
import {
  GetDashboardReportResponseSchema,
  MetricPointSchema,
  NamedCountSchema,
  ReportService,
} from '@moonbase/api-client'
import { describe, expect, it } from 'vitest'

// The web app's data layer is "generated client + a Transport". This test uses
// createRouterTransport — an in-memory server — so the whole contract
// (schemas, service descriptor, client call path) is exercised with no network
// and no browser. Components stay dumb; this seam is what's worth testing.
function newFakeBackend() {
  const transport = createRouterTransport(({ service }) => {
    service(ReportService, {
      getDashboardReport: (req) =>
        create(GetDashboardReportResponseSchema, {
          totalUsers: 42n,
          activeUsers: 40n,
          newUsers: BigInt(req.days),
          activeSessions: 7n,
          userSignups: [create(MetricPointSchema, { date: '2026-01-01', count: 3n })],
          usersByRole: [create(NamedCountSchema, { label: 'admin', count: 1n })],
        }),
    })
  })

  return { transport }
}

describe('ReportService contract via generated client', () => {
  it('returns dashboard aggregates through the generated schemas', async () => {
    const { transport } = newFakeBackend()
    const client = createClient(ReportService, transport)

    const res = await client.getDashboardReport({ days: 30 })

    expect(res.totalUsers).toBe(42n)
    expect(res.newUsers).toBe(30n)
    expect(res.userSignups[0]?.date).toBe('2026-01-01')
    expect(res.usersByRole[0]?.label).toBe('admin')
  })
})
