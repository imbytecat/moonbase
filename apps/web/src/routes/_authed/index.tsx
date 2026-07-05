import { Line, Pie } from '@ant-design/plots'
import { useQuery } from '@connectrpc/connect-query'
import {
  type GetDashboardReportResponse,
  getDashboardReport,
  getMe,
  type MetricPoint,
  type NamedCount,
  Permission,
} from '@moonbase/api-client'
import { createFileRoute } from '@tanstack/react-router'
import {
  Card,
  Col,
  Empty,
  Row,
  Segmented,
  Skeleton,
  Statistic,
  Table,
  Typography,
  theme,
} from 'antd'
import { useState } from 'react'
import { hasPermission } from '#lib/session'
import { m } from '#paraglide/messages.js'
import { useThemeMode } from '#providers/theme-mode'

export const Route = createFileRoute('/_authed/')({
  component: Dashboard,
})

const RANGES = [7, 30, 90] as const

function Dashboard() {
  const { data: meData } = useQuery(getMe)
  const canRead = hasPermission(meData?.user, Permission.REPORT_READ)
  const [days, setDays] = useState<number>(30)

  const { data } = useQuery(getDashboardReport, { days }, { enabled: canRead })

  return (
    <div className="mx-auto max-w-6xl space-y-4">
      <Card>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <Typography.Title level={3} className="!mb-1">
              {m.dashboard_title()}
            </Typography.Title>
            <Typography.Paragraph type="secondary" className="!mb-0">
              {m.dashboard_subtitle()}
            </Typography.Paragraph>
          </div>
          {canRead ? (
            <Segmented
              value={days}
              onChange={setDays}
              options={RANGES.map((value) => ({
                value,
                label: m.report_lastDays({ days: value }),
              }))}
            />
          ) : null}
        </div>
      </Card>

      {canRead ? <ReportBody data={data} days={days} /> : null}
    </div>
  )
}

function ReportBody({
  data,
  days,
}: {
  data: GetDashboardReportResponse | undefined
  days: number
}) {
  const stats = [
    { title: m.report_totalUsers(), value: data?.totalUsers },
    { title: m.report_activeUsers(), value: data?.activeUsers },
    { title: m.report_newUsers(), value: data?.newUsers },
    { title: m.report_activeSessions(), value: data?.activeSessions },
  ]

  return (
    <>
      <Row gutter={[16, 16]}>
        {stats.map((s) => (
          <Col key={s.title} xs={12} lg={6}>
            <Card>
              <Statistic
                title={s.title}
                value={Number(s.value ?? 0)}
                loading={data === undefined}
              />
            </Card>
          </Col>
        ))}
      </Row>

      <Card title={m.report_activityTitle()}>
        {data === undefined ? (
          <Skeleton active title={false} paragraph={{ rows: 6 }} />
        ) : (
          <ActivityChart days={days} signups={data.userSignups} logins={data.logins} />
        )}
      </Card>

      <Row gutter={[16, 16]}>
        <Col xs={24} lg={8}>
          <BreakdownCard title={m.report_usersByRole()} slices={data?.usersByRole} />
        </Col>
        <Col xs={24} lg={8}>
          <BreakdownCard title={m.report_workflowStatus()} slices={data?.workflowRunsByStatus} />
        </Col>
        <Col xs={24} lg={8}>
          <BreakdownCard title={m.report_identityProviders()} slices={data?.identitiesByProvider} />
        </Col>
      </Row>

      <Card title={m.report_detailTitle()} styles={{ body: { padding: 0 } }}>
        <DailyDetailTable
          days={days}
          signups={data?.userSignups ?? []}
          logins={data?.logins ?? []}
          loading={data === undefined}
        />
      </Card>
    </>
  )
}

function useChartTheme() {
  const { resolved } = useThemeMode()
  return resolved === 'dark' ? 'classicDark' : 'classic'
}

// Series are sparse (days with zero activity are omitted by the backend);
// charting needs every day of the window so lines don't skip gaps.
function fillDaily(days: number, points: MetricPoint[], type: string) {
  const byDate = new Map(points.map((p) => [p.date, Number(p.count)]))
  const out: { date: string; type: string; value: number }[] = []
  const cursor = new Date()
  cursor.setDate(cursor.getDate() - (days - 1))
  for (let i = 0; i < days; i++) {
    const date = cursor.toISOString().slice(0, 10)
    out.push({ date, type, value: byDate.get(date) ?? 0 })
    cursor.setDate(cursor.getDate() + 1)
  }
  return out
}

function ActivityChart({
  days,
  signups,
  logins,
}: {
  days: number
  signups: MetricPoint[]
  logins: MetricPoint[]
}) {
  const { token } = theme.useToken()
  const chartTheme = useChartTheme()

  const chartData = [
    ...fillDaily(days, signups, m.report_signups()),
    ...fillDaily(days, logins, m.report_logins()),
  ]

  return (
    <Line
      data={chartData}
      xField="date"
      yField="value"
      colorField="type"
      height={300}
      theme={chartTheme}
      shapeField="smooth"
      scale={{ color: { range: [token.colorPrimary, token.colorSuccess] } }}
      axis={{ y: { tickCount: 5 } }}
      legend={{ color: { position: 'top' } }}
      animate={false}
    />
  )
}

function DailyDetailTable({
  days,
  signups,
  logins,
  loading,
}: {
  days: number
  signups: MetricPoint[]
  logins: MetricPoint[]
  loading: boolean
}) {
  const signupsByDate = new Map(signups.map((p) => [p.date, Number(p.count)]))
  const loginsByDate = new Map(logins.map((p) => [p.date, Number(p.count)]))

  const rows = fillDaily(days, [], '')
    .map(({ date }) => ({
      date,
      signups: signupsByDate.get(date) ?? 0,
      logins: loginsByDate.get(date) ?? 0,
    }))
    .reverse()

  return (
    <Table
      rowKey="date"
      dataSource={rows}
      loading={loading}
      size="middle"
      pagination={{ pageSize: 10, hideOnSinglePage: true, showSizeChanger: false }}
      scroll={{ x: 'max-content' }}
      columns={[
        { title: m.report_date(), dataIndex: 'date' },
        {
          title: m.report_signups(),
          dataIndex: 'signups',
          align: 'right',
          sorter: (a, b) => a.signups - b.signups,
        },
        {
          title: m.report_logins(),
          dataIndex: 'logins',
          align: 'right',
          sorter: (a, b) => a.logins - b.logins,
        },
      ]}
    />
  )
}

function BreakdownCard({ title, slices }: { title: string; slices: NamedCount[] | undefined }) {
  const chartTheme = useChartTheme()

  if (slices === undefined) {
    return (
      <Card title={title}>
        <Skeleton active title={false} paragraph={{ rows: 5 }} />
      </Card>
    )
  }

  const data = slices
    .filter((s) => s.count > 0n)
    .map((s) => ({ label: s.label, value: Number(s.count) }))

  return (
    <Card title={title}>
      {data.length === 0 ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} />
      ) : (
        <Pie
          data={data}
          angleField="value"
          colorField="label"
          height={240}
          theme={chartTheme}
          innerRadius={0.6}
          label={{ text: 'value', style: { fontWeight: 'bold' } }}
          legend={{ color: { position: 'bottom', layout: { justifyContent: 'center' } } }}
          animate={false}
        />
      )}
    </Card>
  )
}
