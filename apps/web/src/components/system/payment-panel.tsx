import { AlipayCircleOutlined, WechatOutlined } from '@ant-design/icons'
import { useMutation } from '@connectrpc/connect-query'
import {
  bindPaymentPurpose,
  deletePaymentProfile,
  type PaymentSettings,
  type Profile,
} from '@moonbase/api-client'
import { App } from 'antd'
import { useState } from 'react'
import { ProfileManager, ProviderTag } from '#components/profile-manager'
import { PaymentProfileDrawer } from '#components/system/payment-profile-drawer'
import { humanizeError } from '#lib/errors'
import { m } from '#paraglide/messages.js'

const PURPOSE_LABELS: Record<string, () => string> = {
  checkout: m.systemPage_paymentPurposeCheckout,
}

const PROVIDER_NAMES: Record<string, () => string> = {
  alipay: m.systemPage_providerAlipay,
  wechat: m.systemPage_providerWechatPay,
}

export function PaymentPanel({
  payment,
  onChanged,
}: {
  payment: PaymentSettings | undefined
  onChanged: () => void
}) {
  const { message } = App.useApp()
  const profiles = payment?.profiles ?? []
  const bindings = payment?.bindings ?? []
  const [editing, setEditing] = useState<Profile | 'new' | undefined>()

  const deleteMutation = useMutation(deletePaymentProfile, {
    onSuccess: () => {
      onChanged()
      message.success(m.systemPage_profileDeleted())
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const bindMutation = useMutation(bindPaymentPurpose, {
    onSuccess: () => {
      onChanged()
      message.success(m.systemPage_saved())
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const descriptionOf = (p: Profile) => {
    switch (p.provider) {
      case 'alipay':
        return String(p.config?.appId ?? '')
      case 'wechat':
        return String(p.config?.mchId ?? '')
      default:
        return ''
    }
  }

  return (
    <>
      <ProfileManager
        profiles={profiles}
        bindings={bindings.map((b) => ({
          purpose: b.purpose,
          profileIds: b.profileIds,
          multiple: true,
        }))}
        texts={{
          profilesTitle: m.systemPage_paymentProfilesTitle(),
          profilesHint: m.systemPage_paymentProfilesHint(),
          noProfiles: m.systemPage_paymentNoProfiles(),
          confirmDelete: m.systemPage_confirmDeleteProfile(),
          bindingsHint: m.systemPage_paymentBindingsHint(),
        }}
        purposeLabel={(purpose) => PURPOSE_LABELS[purpose]?.() ?? purpose}
        profileIcon={(p) =>
          p.provider === 'alipay' ? (
            <AlipayCircleOutlined className="text-lg text-(--ant-color-info)" />
          ) : (
            <WechatOutlined className="text-lg text-(--ant-color-success)" />
          )
        }
        profileTags={(p) => <ProviderTag name={PROVIDER_NAMES[p.provider]?.() ?? p.provider} />}
        profileDescription={(p) => descriptionOf(p)}
        onAdd={() => setEditing('new')}
        onEdit={(p) => setEditing(p)}
        onDelete={(p) => deleteMutation.mutate({ id: p.id })}
        deleting={deleteMutation.isPending}
        onBind={(purpose, ids) => bindMutation.mutate({ purpose, profileIds: ids })}
        binding={bindMutation.isPending}
      />

      <PaymentProfileDrawer
        key={editing === 'new' ? 'new' : (editing?.id ?? 'closed')}
        profile={editing === 'new' ? undefined : editing}
        open={editing !== undefined}
        onClose={() => setEditing(undefined)}
        onChanged={() => {
          setEditing(undefined)
          onChanged()
        }}
      />
    </>
  )
}
