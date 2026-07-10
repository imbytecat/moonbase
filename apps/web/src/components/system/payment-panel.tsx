import { useMutation, useQuery } from '@connectrpc/connect-query'
import {
  bindPaymentPurpose,
  deletePaymentProfile,
  describePaymentProviders,
  type PaymentSettings,
  type Profile,
} from '@moonbase/api-client'
import { App } from 'antd'
import { ProfileManager } from '#components/profile-manager'
import { PaymentProfileDrawer } from '#components/system/payment-profile-drawer'
import { humanizeError } from '#lib/errors'
import { useEditingTarget } from '#lib/use-editing-target'

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
  const drawer = useEditingTarget<Profile>()
  const { data: describe } = useQuery(describePaymentProviders, {})

  const deleteMutation = useMutation(deletePaymentProfile, {
    onSuccess: () => {
      onChanged()
      message.success('存储配置已删除')
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const bindMutation = useMutation(bindPaymentPurpose, {
    onSuccess: () => {
      onChanged()
      message.success('设置已保存')
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  return (
    <>
      <ProfileManager
        profiles={profiles}
        bindings={bindings.map((b) => ({
          purpose: b.purpose,
          profileIds: b.profileIds,
        }))}
        purposes={describe?.purposes ?? []}
        providers={describe?.providers ?? []}
        texts={{
          profilesTitle: '支付配置',
          profilesHint: '可添加多个支付渠道，例如支付宝和微信支付，绑定后同时作为收银台选项',
          noProfiles: '尚未添加支付配置',
          confirmDelete: '删除该存储配置？',
          bindingsHint: '为每个收款场景选择支付渠道，可多选，付款人在收银台自行选择',
        }}
        onAdd={drawer.add}
        onEdit={drawer.edit}
        onDelete={(p) => deleteMutation.mutate({ id: p.id })}
        deleting={deleteMutation.isPending}
        onBind={(purpose, ids) => bindMutation.mutate({ purpose, profileIds: ids })}
        binding={bindMutation.isPending}
      />

      <PaymentProfileDrawer
        key={drawer.drawerKey}
        profile={drawer.profile}
        open={drawer.open}
        onClose={drawer.close}
        onChanged={() => {
          drawer.close()
          onChanged()
        }}
      />
    </>
  )
}
