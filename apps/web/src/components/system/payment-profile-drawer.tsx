import { AlipayCircleOutlined, WechatOutlined } from '@ant-design/icons'
import type { JsonObject } from '@bufbuild/protobuf'
import { useMutation, useQuery } from '@connectrpc/connect-query'
import {
  createPaymentProfile,
  describePaymentProviders,
  type Profile,
  type ProviderForm,
  updatePaymentProfile,
} from '@moonbase/api-client'
import { App } from 'antd'
import { useState } from 'react'
import { ProfileFormDrawer, type ProviderOption } from '#components/profile-form-drawer'
import { ConfigForm } from '#components/system/config-form'
import { humanizeError } from '#lib/errors'
import { methodDesc, methodLabel } from '#lib/payments'

export function PaymentProfileDrawer({
  profile,
  open,
  onClose,
  onChanged,
}: {
  profile: Profile | undefined
  open: boolean
  onClose: () => void
  onChanged: () => void
}) {
  const { message } = App.useApp()
  const [dirty, setDirty] = useState(false)

  const { data: describe } = useQuery(describePaymentProviders, {})
  const forms = describe?.providers ?? {}

  const createMutation = useMutation(createPaymentProfile, {
    onSuccess: () => {
      message.success('存储配置已创建')
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })
  const updateMutation = useMutation(updatePaymentProfile, {
    onSuccess: () => {
      message.success('设置已保存')
      onChanged()
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const providers: ProviderOption[] = [
    {
      value: 'alipay',
      label: '支付宝',
      description: '支付宝开放平台商户收款（当面付/手机网站/小程序）',
      icon: <AlipayCircleOutlined className="text-xl text-(--ant-color-info)" />,
    },
    {
      value: 'wechat',
      label: '微信支付',
      description: '微信支付商户收款（APIv3：Native/H5/JSAPI）',
      icon: <WechatOutlined className="text-xl text-(--ant-color-success)" />,
    },
  ]

  return (
    <ProfileFormDrawer
      open={open}
      onClose={onClose}
      dirty={dirty}
      profileProvider={profile?.provider}
      providers={providers}
    >
      {(provider) => {
        const providerForm = forms[provider]
        if (!providerForm) return null
        return (
          <ConfigForm
            key={provider}
            providerForm={withMethodLabels(providerForm)}
            provider={provider}
            profile={profile}
            saving={createMutation.isPending || updateMutation.isPending}
            onDirtyChange={setDirty}
            onSubmit={(p) => {
              if (profile) updateMutation.mutate({ profile: p })
              else createMutation.mutate({ profile: p })
            }}
          />
        )
      }}
    </ProfileFormDrawer>
  )
}

interface MethodOption {
  const: string
  title?: string
}
interface MethodsView {
  properties?: { methods?: { items?: { oneOf?: MethodOption[] } } }
}

// Methods are ADR-0009's fixed-catalog exception: the driver ships value-only
// options and the Chinese copy lives in payments.ts (same source as checkout).
// Fill the labels/descriptions into the JSON Schema before rjsf renders it.
function withMethodLabels(providerForm: ProviderForm): ProviderForm {
  const schema = structuredClone(providerForm.schema ?? {}) as JsonObject
  const oneOf = (schema as unknown as MethodsView).properties?.methods?.items?.oneOf
  if (!oneOf) return providerForm
  const descriptions: Record<string, string> = {}
  for (const option of oneOf) {
    option.title = methodLabel(option.const)
    const desc = methodDesc(option.const)
    if (desc) descriptions[option.const] = desc
  }
  const uiSchema = structuredClone(providerForm.uiSchema ?? {}) as JsonObject
  const current = (uiSchema.methods as JsonObject | undefined) ?? {}
  const options = (current['ui:options'] as JsonObject | undefined) ?? {}
  uiSchema.methods = { ...current, 'ui:options': { ...options, descriptions } }
  return { ...providerForm, schema, uiSchema }
}
