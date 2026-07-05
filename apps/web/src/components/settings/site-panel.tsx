import { useMutation } from '@connectrpc/connect-query'
import { presignSiteAssetUpload, type SiteSettings } from '@moonbase/api-client'
import { App, Button, Form, Input, Upload } from 'antd'
import { humanizeError } from '#lib/errors'
import { uploadToPresignedUrl } from '#lib/upload'
import { m } from '#paraglide/messages.js'

interface SiteFormValues {
  name: string
  description: string
  copyright: string
  icpBeian: string
}

export function SitePanel({
  site,
  saving,
  onSave,
}: {
  site: SiteSettings | undefined
  saving: boolean
  onSave: (site: SiteFormValues & { logoFileId: string; faviconFileId: string }) => void
}) {
  const [form] = Form.useForm<SiteFormValues>()

  return (
    <Form
      form={form}
      layout="vertical"
      requiredMark={false}
      initialValues={{
        name: site?.name ?? '',
        description: site?.description ?? '',
        copyright: site?.copyright ?? '',
        icpBeian: site?.icpBeian ?? '',
      }}
      onFinish={(values) =>
        onSave({
          ...values,
          logoFileId: site?.logoFileId ?? '',
          faviconFileId: site?.faviconFileId ?? '',
        })
      }
    >
      <div className="grid grid-cols-2 gap-4">
        <Form.Item name="name" label={m.settingsPage_siteName()}>
          <Input placeholder={m.common_appName()} />
        </Form.Item>
        <Form.Item name="description" label={m.settingsPage_siteDescription()}>
          <Input />
        </Form.Item>
      </div>

      <div className="mb-6 grid grid-cols-2 gap-4">
        <BrandAssetField
          kind="logo"
          label={m.settingsPage_siteLogo()}
          hint={m.settingsPage_siteLogoHint()}
          currentFileId={site?.logoFileId ?? ''}
          accept="image/png,image/jpeg,image/webp,image/svg+xml"
          onUploaded={(fileId) =>
            onSave({
              ...form.getFieldsValue(),
              logoFileId: fileId,
              faviconFileId: site?.faviconFileId ?? '',
            })
          }
          onClear={() =>
            onSave({
              ...form.getFieldsValue(),
              logoFileId: '',
              faviconFileId: site?.faviconFileId ?? '',
            })
          }
        />
        <BrandAssetField
          kind="favicon"
          label={m.settingsPage_siteFavicon()}
          hint={m.settingsPage_siteFaviconHint()}
          currentFileId={site?.faviconFileId ?? ''}
          accept="image/png,image/svg+xml,image/x-icon,image/vnd.microsoft.icon"
          onUploaded={(fileId) =>
            onSave({
              ...form.getFieldsValue(),
              faviconFileId: fileId,
              logoFileId: site?.logoFileId ?? '',
            })
          }
          onClear={() =>
            onSave({
              ...form.getFieldsValue(),
              faviconFileId: '',
              logoFileId: site?.logoFileId ?? '',
            })
          }
        />
      </div>

      <div className="grid grid-cols-2 gap-4">
        <Form.Item name="copyright" label={m.settingsPage_siteCopyright()}>
          <Input placeholder="© 2026 Acme Inc." />
        </Form.Item>
        <Form.Item name="icpBeian" label={m.settingsPage_siteIcp()}>
          <Input />
        </Form.Item>
      </div>

      <Button type="primary" htmlType="submit" loading={saving}>
        {m.common_save()}
      </Button>
    </Form>
  )
}

function BrandAssetField({
  kind,
  label,
  hint,
  currentFileId,
  accept,
  onUploaded,
  onClear,
}: {
  kind: 'logo' | 'favicon'
  label: string
  hint: string
  currentFileId: string
  accept: string
  onUploaded: (fileId: string) => void
  onClear: () => void
}) {
  const { message } = App.useApp()
  const presignMutation = useMutation(presignSiteAssetUpload)

  const upload = async (file: File) => {
    try {
      const presigned = await presignMutation.mutateAsync({
        kind,
        contentType: file.type,
        contentLength: BigInt(file.size),
      })
      await uploadToPresignedUrl(presigned.uploadUrl, file)
      onUploaded(presigned.fileId)
    } catch (err) {
      message.error(humanizeError(err))
    }
  }

  return (
    <div>
      <div className="mb-2 text-sm">{label}</div>
      <div className="flex items-center gap-2">
        <Upload
          accept={accept}
          showUploadList={false}
          beforeUpload={(file) => {
            void upload(file)
            return false
          }}
        >
          <Button loading={presignMutation.isPending}>
            {currentFileId ? m.settingsPage_replaceAsset() : m.settingsPage_uploadAsset()}
          </Button>
        </Upload>
        {currentFileId ? (
          <Button type="text" danger onClick={onClear}>
            {m.settingsPage_clearAsset()}
          </Button>
        ) : null}
      </div>
      <div className="mt-1 text-xs text-(--ant-color-text-tertiary)">{hint}</div>
    </div>
  )
}
