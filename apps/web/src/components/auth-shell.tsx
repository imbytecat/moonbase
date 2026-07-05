import { GlobalOutlined } from '@ant-design/icons'
import { useQuery } from '@connectrpc/connect-query'
import { getSiteInfo } from '@moonbase/api-client'
import { Button, Card, Dropdown, Typography } from 'antd'
import type { ReactNode } from 'react'
import { LOCALE_LABELS } from '#lib/locale'
import { siteName } from '#lib/site'
import { getLocale, locales, setLocale } from '#paraglide/runtime.js'

// Shared shell for the public auth pages (login/register/forgot/reset/verify):
// centered card, site branding on top, legal footer at the bottom. Brand data
// comes from the public GetSiteInfo query, so a fresh deploy shows the
// built-in name until an admin customizes it.
export function AuthShell({ subtitle, children }: { subtitle?: string; children: ReactNode }) {
  const { data: siteInfo } = useQuery(getSiteInfo)

  return (
    <div className="relative flex min-h-screen flex-col items-center justify-center bg-(--ant-color-bg-layout) p-4">
      {locales.length > 1 ? (
        <Dropdown
          trigger={['click']}
          menu={{
            selectable: true,
            selectedKeys: [getLocale()],
            items: locales.map((locale) => ({
              key: locale,
              label: LOCALE_LABELS[locale],
              onClick: () => setLocale(locale),
            })),
          }}
        >
          <Button type="text" icon={<GlobalOutlined />} className="!absolute top-4 right-4">
            {LOCALE_LABELS[getLocale()]}
          </Button>
        </Dropdown>
      ) : null}
      <Card className="w-full max-w-md shadow-sm">
        <div className="mb-6 text-center">
          {siteInfo?.logoUrl ? (
            <img src={siteInfo.logoUrl} alt="" className="mx-auto mb-3 size-12 object-contain" />
          ) : null}
          <Typography.Title level={3} className="!mb-1">
            {siteName(siteInfo)}
          </Typography.Title>
          {subtitle || siteInfo?.description ? (
            <Typography.Text type="secondary">{subtitle ?? siteInfo?.description}</Typography.Text>
          ) : null}
        </div>
        {children}
      </Card>

      {siteInfo?.copyright || siteInfo?.icpBeian ? (
        <div className="mt-6 space-x-3 text-center text-xs text-(--ant-color-text-quaternary)">
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
      ) : null}
    </div>
  )
}
