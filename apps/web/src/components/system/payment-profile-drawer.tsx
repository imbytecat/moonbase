import { useMutation, useQuery } from '@connectrpc/connect-query'
import {
  createPaymentProfile,
  describePaymentProviders,
  type Profile,
  updatePaymentProfile,
} from '@moonbase/api-client'
import { App } from 'antd'
import { useState } from 'react'
import { ProfileFormDrawer } from '#components/profile-form-drawer'
import { ConfigForm } from '#components/system/config-form'
import { humanizeError } from '#lib/errors'

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
  const providers = describe?.providers ?? []

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

  return (
    <ProfileFormDrawer
      open={open}
      onClose={onClose}
      dirty={dirty}
      profileProvider={profile?.provider}
      providers={providers}
    >
      {(provider) => {
        const providerForm = providers.find((item) => item.key === provider)?.config
        if (!providerForm) return null
        return (
          <ConfigForm
            key={provider}
            providerForm={providerForm}
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
