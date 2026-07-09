import { createConnectQueryKey, useMutation } from '@connectrpc/connect-query'
import { getAuthConfig, getSettings, updateSettings } from '@moonbase/api-client'
import { useQueryClient } from '@tanstack/react-query'
import { App } from 'antd'
import { humanizeError } from '#lib/errors'
import { siteInfoQueryKey } from '#lib/site'

export function useUpdateSettings() {
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  return useMutation(updateSettings, {
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: createConnectQueryKey({ schema: getSettings, cardinality: 'finite' }),
      })
      void queryClient.invalidateQueries({
        queryKey: createConnectQueryKey({ schema: getAuthConfig, cardinality: 'finite' }),
      })
      // Site identity feeds the shell/login/head through the public
      // GetSiteInfo query — refresh it so branding changes apply live.
      void queryClient.invalidateQueries({ queryKey: siteInfoQueryKey() })
      message.success('设置已保存')
    },
    onError: (err) => message.error(humanizeError(err)),
  })
}
